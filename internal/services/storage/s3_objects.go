package storage

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// Spooling keeps memory bounded and gives the SDK a seekable body for retries.
// Current request limits are small; multipart uploads are unnecessary.
func (s *S3Storage) PutNew(ctx context.Context, key string, src io.Reader) (ObjectInfo, error) {
	p, err := objectPath(s.tenantID, key)
	if err != nil {
		return ObjectInfo{}, err
	}
	f, err := os.CreateTemp("", "clientshare-object-*")
	if err != nil {
		return ObjectInfo{}, err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	info, err := Hash(ctx, io.TeeReader(src, f))
	if err != nil {
		return ObjectInfo{}, err
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return ObjectInfo{}, err
	}
	digest, _ := hex.DecodeString(info.Checksum)
	_, err = s.client.PutObject(ctx, &awss3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(p), Body: f, ContentLength: aws.Int64(info.Size), IfNoneMatch: aws.String("*"), ChecksumSHA256: aws.String(base64.StdEncoding.EncodeToString(digest))})
	return info, translateS3Error(err)
}

func (s *S3Storage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	p, err := objectPath(s.tenantID, key)
	if err != nil {
		return nil, err
	}
	result, err := s.client.GetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(p)})
	if err != nil {
		return nil, translateS3Error(err)
	}
	return result.Body, nil
}

func (s *S3Storage) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	f, err := s.Open(ctx, key)
	if err != nil {
		return ObjectInfo{}, err
	}
	info, err := Hash(ctx, f)
	return info, errors.Join(err, f.Close())
}

func (s *S3Storage) DeleteObject(ctx context.Context, key string) error {
	p, err := objectPath(s.tenantID, key)
	if err != nil {
		return err
	}
	_, err = s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(p)})
	return translateS3Error(err)
}
