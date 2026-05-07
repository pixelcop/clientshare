package storage

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	pathpkg "path"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	awss3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

type S3Config struct {
	Bucket       string
	Region       string
	Endpoint     string
	AccessKey    string
	SecretKey    string
	UsePathStyle bool
}

type S3Storage struct {
	client *awss3.Client
	bucket string
	prefix string
}

func NewS3Storage(ctx context.Context, cfg S3Config) (*S3Storage, error) {
	cfg.Bucket = strings.TrimSpace(cfg.Bucket)
	cfg.Region = strings.TrimSpace(cfg.Region)
	cfg.Endpoint = strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")

	if cfg.Bucket == "" {
		return nil, fmt.Errorf("s3 bucket must be set")
	}
	if cfg.Region == "" {
		return nil, fmt.Errorf("s3 region must be set")
	}
	if (cfg.AccessKey == "") != (cfg.SecretKey == "") {
		return nil, fmt.Errorf("s3 access key and secret key must be set together")
	}

	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}
	if cfg.AccessKey != "" {
		loadOptions = append(loadOptions,
			awsconfig.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
			),
		)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load s3 config: %w", err)
	}

	client := awss3.NewFromConfig(awsCfg, func(options *awss3.Options) {
		options.UsePathStyle = cfg.UsePathStyle
		if cfg.Endpoint != "" {
			options.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	})

	if _, err := client.HeadBucket(ctx, &awss3.HeadBucketInput{Bucket: aws.String(cfg.Bucket)}); err != nil {
		return nil, fmt.Errorf("verify s3 bucket %q: %w", cfg.Bucket, translateS3Error(err))
	}

	return &S3Storage{client: client, bucket: cfg.Bucket}, nil
}

func (s *S3Storage) Tenant(tenantID string) Storage {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return s
	}
	return &S3Storage{
		client: s.client,
		bucket: s.bucket,
		prefix: joinObjectPath(s.prefix, tenantRootPath(tenantID)),
	}
}

func (s *S3Storage) List(path string) ([]FileInfo, error) {
	ctx := context.Background()
	key, err := s.objectKey(path)
	if err != nil {
		return nil, err
	}
	prefix := s.listPrefixForKey(key)

	entries := map[string]FileInfo{}
	paginator := awss3.NewListObjectsV2Paginator(s.client, &awss3.ListObjectsV2Input{
		Bucket:    aws.String(s.bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, translateS3Error(err)
		}

		for _, commonPrefix := range page.CommonPrefixes {
			name := strings.TrimSuffix(strings.TrimPrefix(aws.ToString(commonPrefix.Prefix), prefix), "/")
			if name == "" {
				continue
			}
			entries[name] = FileInfo{Name: name, IsDir: true}
		}

		for _, object := range page.Contents {
			fileInfo, ok := listObjectToFileInfo(prefix, object)
			if !ok {
				continue
			}
			if existing, exists := entries[fileInfo.Name]; exists && existing.IsDir {
				continue
			}
			entries[fileInfo.Name] = fileInfo
		}
	}

	logicalPath, err := sanitizeObjectPath(path)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		if logicalPath == "" {
			return []FileInfo{}, nil
		}
		dirExists, err := s.dirExists(ctx, key)
		if err != nil {
			return nil, err
		}
		if dirExists {
			return []FileInfo{}, nil
		}
		return nil, os.ErrNotExist
	}

	files := make([]FileInfo, 0, len(entries))
	for _, fileInfo := range entries {
		files = append(files, fileInfo)
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})
	return files, nil
}

func (s *S3Storage) Upload(dst string, src io.Reader) (int64, error) {
	key, err := s.objectKey(dst)
	if err != nil {
		return 0, err
	}
	if key == "" {
		return 0, fmt.Errorf("%w: destination path is required", os.ErrPermission)
	}

	data, err := io.ReadAll(src)
	if err != nil {
		return 0, err
	}

	_, err = s.client.PutObject(context.Background(), &awss3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		IfNoneMatch: aws.String("*"),
	})
	if err != nil {
		return 0, translateS3Error(err)
	}

	return int64(len(data)), nil
}

func (s *S3Storage) Download(src string) (io.ReadCloser, error) {
	key, err := s.objectKey(src)
	if err != nil {
		return nil, err
	}
	if key == "" {
		return nil, os.ErrNotExist
	}

	object, err := s.client.GetObject(context.Background(), &awss3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, translateS3Error(err)
	}
	return object.Body, nil
}

func (s *S3Storage) Delete(path string) error {
	ctx := context.Background()
	key, err := s.objectKey(path)
	if err != nil {
		return err
	}
	if key == "" {
		return fmt.Errorf("%w: refusing to delete storage root", os.ErrPermission)
	}

	if err := s.deleteAll(ctx, s.listPrefixForKey(key)); err != nil {
		return err
	}
	_, err = s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return translateS3Error(err)
	}
	return nil
}

func (s *S3Storage) CreateFolder(path string) error {
	ctx := context.Background()
	key, err := s.objectKey(path)
	if err != nil {
		return err
	}
	if key == "" {
		return nil
	}

	exists, err := s.objectExists(ctx, key)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%w: object already exists at %q", os.ErrExist, path)
	}

	_, err = s.client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(ensureTrailingSlash(key)),
		Body:   bytes.NewReader(nil),
	})
	if err != nil {
		return translateS3Error(err)
	}
	return nil
}

func (s *S3Storage) Move(src string, dst string) error {
	ctx := context.Background()
	srcKey, err := s.objectKey(src)
	if err != nil {
		return err
	}
	dstKey, err := s.objectKey(dst)
	if err != nil {
		return err
	}
	if srcKey == "" || dstKey == "" {
		return fmt.Errorf("%w: cannot move storage root", os.ErrPermission)
	}
	if srcKey == dstKey {
		return nil
	}

	srcPrefix := s.listPrefixForKey(srcKey)
	dstPrefix := s.listPrefixForKey(dstKey)
	if strings.HasPrefix(dstPrefix, srcPrefix) {
		return fmt.Errorf("%w: cannot move %q into itself", os.ErrPermission, src)
	}

	dirKeys, err := s.listAllKeys(ctx, srcPrefix)
	if err != nil {
		return err
	}
	if len(dirKeys) > 0 {
		dstExists, err := s.pathExists(ctx, dstKey)
		if err != nil {
			return err
		}
		if dstExists {
			return fmt.Errorf("%w: destination %q already exists", os.ErrExist, dst)
		}
		for _, sourceKey := range dirKeys {
			relative := strings.TrimPrefix(sourceKey, srcPrefix)
			targetKey := dstPrefix + relative
			if sourceKey == srcPrefix {
				targetKey = dstPrefix
			}
			if strings.HasSuffix(sourceKey, "/") {
				if _, err := s.client.PutObject(ctx, &awss3.PutObjectInput{
					Bucket: aws.String(s.bucket),
					Key:    aws.String(targetKey),
					Body:   bytes.NewReader(nil),
				}); err != nil {
					return translateS3Error(err)
				}
				continue
			}
			if err := s.copyObject(ctx, sourceKey, targetKey); err != nil {
				return err
			}
		}
		return s.deleteKeys(ctx, dirKeys)
	}

	srcExists, err := s.objectExists(ctx, srcKey)
	if err != nil {
		return err
	}
	if !srcExists {
		return os.ErrNotExist
	}

	dstExists, err := s.pathExists(ctx, dstKey)
	if err != nil {
		return err
	}
	if dstExists {
		return fmt.Errorf("%w: destination %q already exists", os.ErrExist, dst)
	}

	if err := s.copyObject(ctx, srcKey, dstKey); err != nil {
		return err
	}
	_, err = s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(srcKey),
	})
	if err != nil {
		return translateS3Error(err)
	}
	return nil
}

func (s *S3Storage) ZipFolder(path string, dst string) error {
	ctx := context.Background()
	key, err := s.objectKey(path)
	if err != nil {
		return err
	}
	prefix := s.listPrefixForKey(key)
	keys, err := s.listAllKeys(ctx, prefix)
	if err != nil {
		return err
	}

	logicalPath, err := sanitizeObjectPath(path)
	if err != nil {
		return err
	}
	if len(keys) == 0 && logicalPath != "" {
		exists, err := s.dirExists(ctx, key)
		if err != nil {
			return err
		}
		if !exists {
			return os.ErrNotExist
		}
	}

	zipFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, objectKey := range keys {
		if strings.HasSuffix(objectKey, "/") {
			continue
		}

		relative := strings.TrimPrefix(objectKey, prefix)
		if relative == "" {
			continue
		}

		object, err := s.client.GetObject(ctx, &awss3.GetObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(objectKey),
		})
		if err != nil {
			return translateS3Error(err)
		}

		writer, err := zipWriter.Create(relative)
		if err != nil {
			object.Body.Close()
			return err
		}
		if _, err := io.Copy(writer, object.Body); err != nil {
			object.Body.Close()
			return err
		}
		if err := object.Body.Close(); err != nil {
			return err
		}
	}

	return nil
}

func (s *S3Storage) objectKey(path string) (string, error) {
	cleaned, err := sanitizeObjectPath(path)
	if err != nil {
		return "", err
	}
	return joinObjectPath(s.prefix, cleaned), nil
}

func (s *S3Storage) listPrefixForKey(key string) string {
	if key == "" {
		return ensureTrailingSlash(s.prefix)
	}
	return ensureTrailingSlash(key)
}

func (s *S3Storage) objectExists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}
	if isS3NotFound(err) {
		return false, nil
	}
	return false, translateS3Error(err)
}

func (s *S3Storage) dirExists(ctx context.Context, key string) (bool, error) {
	if key == "" {
		return true, nil
	}

	markerExists, err := s.objectExists(ctx, ensureTrailingSlash(key))
	if err != nil {
		return false, err
	}
	if markerExists {
		return true, nil
	}

	result, err := s.client.ListObjectsV2(ctx, &awss3.ListObjectsV2Input{
		Bucket:  aws.String(s.bucket),
		Prefix:  aws.String(ensureTrailingSlash(key)),
		MaxKeys: aws.Int32(1),
	})
	if err != nil {
		return false, translateS3Error(err)
	}
	return len(result.Contents) > 0, nil
}

func (s *S3Storage) pathExists(ctx context.Context, key string) (bool, error) {
	if key == "" {
		return true, nil
	}

	exactExists, err := s.objectExists(ctx, key)
	if err != nil {
		return false, err
	}
	if exactExists {
		return true, nil
	}

	return s.dirExists(ctx, key)
}

func (s *S3Storage) listAllKeys(ctx context.Context, prefix string) ([]string, error) {
	paginator := awss3.NewListObjectsV2Paginator(s.client, &awss3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})
	var keys []string
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, translateS3Error(err)
		}
		for _, object := range page.Contents {
			keys = append(keys, aws.ToString(object.Key))
		}
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *S3Storage) deleteAll(ctx context.Context, prefix string) error {
	keys, err := s.listAllKeys(ctx, prefix)
	if err != nil {
		return err
	}
	return s.deleteKeys(ctx, keys)
}

func (s *S3Storage) deleteKeys(ctx context.Context, keys []string) error {
	for _, key := range keys {
		_, err := s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			return translateS3Error(err)
		}
	}
	return nil
}

func (s *S3Storage) copyObject(ctx context.Context, srcKey, dstKey string) error {
	copySource := s.bucket + "/" + strings.ReplaceAll(url.PathEscape(srcKey), "%2F", "/")

	_, err := s.client.CopyObject(ctx, &awss3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		Key:        aws.String(dstKey),
		CopySource: aws.String(copySource),
	})
	if err != nil {
		return translateS3Error(err)
	}
	return nil
}

func sanitizeObjectPath(path string) (string, error) {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if path == "" || path == "." {
		return "", nil
	}
	if strings.HasPrefix(path, "/") {
		return "", os.ErrPermission
	}

	parts := strings.Split(path, "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			return "", os.ErrPermission
		default:
			cleaned = append(cleaned, part)
		}
	}
	if len(cleaned) == 0 {
		return "", nil
	}
	return pathpkg.Join(cleaned...), nil
}

func joinObjectPath(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		cleaned, err := sanitizeObjectPath(part)
		if err != nil || cleaned == "" {
			continue
		}
		filtered = append(filtered, cleaned)
	}
	if len(filtered) == 0 {
		return ""
	}
	return pathpkg.Join(filtered...)
}

func ensureTrailingSlash(key string) string {
	if key == "" {
		return ""
	}
	if strings.HasSuffix(key, "/") {
		return key
	}
	return key + "/"
}

func listObjectToFileInfo(prefix string, object awss3types.Object) (FileInfo, bool) {
	key := aws.ToString(object.Key)
	if key == prefix {
		return FileInfo{}, false
	}

	relative := strings.TrimPrefix(key, prefix)
	if relative == "" {
		return FileInfo{}, false
	}

	isDir := strings.HasSuffix(relative, "/")
	relative = strings.TrimSuffix(relative, "/")
	if relative == "" {
		return FileInfo{}, false
	}
	if strings.Contains(relative, "/") {
		name := strings.SplitN(relative, "/", 2)[0]
		return FileInfo{Name: name, IsDir: true}, true
	}

	return FileInfo{
		Name:    relative,
		Size:    aws.ToInt64(object.Size),
		ModTime: aws.ToTime(object.LastModified),
		IsDir:   isDir,
	}, true
}

func translateS3Error(err error) error {
	if err == nil {
		return nil
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "NoSuchBucket":
			return fmt.Errorf("%w: %s", os.ErrNotExist, apiErr.ErrorCode())
		case "PreconditionFailed", "ConditionalRequestConflict":
			return fmt.Errorf("%w: %s", os.ErrExist, apiErr.ErrorCode())
		}
	}

	return err
}

func isS3NotFound(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.ErrorCode() {
	case "NoSuchKey", "NotFound", "NoSuchBucket":
		return true
	default:
		return false
	}
}
