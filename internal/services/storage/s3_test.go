package storage_test

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"github.com/stretchr/testify/require"
)

func TestS3StorageTenantIsolation(t *testing.T) {
	store := newFakeS3Storage(t)
	tenantA := store.Tenant("tenant-a")
	tenantB := store.Tenant("tenant-b")

	_, err := tenantA.Upload("Acme/shared.txt", strings.NewReader("tenant-a"))
	require.NoError(t, err)
	_, err = tenantB.Upload("Acme/shared.txt", strings.NewReader("tenant-b"))
	require.NoError(t, err)

	require.Equal(t, "tenant-a", readStoredValue(t, tenantA, "Acme/shared.txt"))
	require.Equal(t, "tenant-b", readStoredValue(t, tenantB, "Acme/shared.txt"))

	_, err = tenantA.Upload("Acme/shared.txt", strings.NewReader("duplicate"))
	require.Error(t, err)
	require.True(t, errors.Is(err, os.ErrExist))

	_, err = tenantA.Download("../escape.txt")
	require.Error(t, err)
	require.True(t, errors.Is(err, os.ErrPermission))
	_, err = tenantA.Upload("../escape.txt", strings.NewReader("nope"))
	require.Error(t, err)
	require.True(t, errors.Is(err, os.ErrPermission))
}

func TestS3StorageFolderLifecycle(t *testing.T) {
	store := newFakeS3Storage(t).Tenant("tenant-a")

	require.NoError(t, store.CreateFolder("Acme"))
	require.NoError(t, store.CreateFolder("Acme/empty"))
	_, err := store.Upload("Acme/docs/report.txt", strings.NewReader("quarterly-report"))
	require.NoError(t, err)

	rootEntries, err := store.List("")
	require.NoError(t, err)
	require.Len(t, rootEntries, 1)
	require.Equal(t, "Acme", rootEntries[0].Name)
	require.True(t, rootEntries[0].IsDir)

	entries, err := store.List("Acme")
	require.NoError(t, err)
	require.Len(t, entries, 2)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	require.Equal(t, "docs", entries[0].Name)
	require.True(t, entries[0].IsDir)
	require.Equal(t, "empty", entries[1].Name)
	require.True(t, entries[1].IsDir)

	require.NoError(t, store.Move("Acme", "Acme Renamed"))
	_, err = store.Download("Acme/docs/report.txt")
	require.Error(t, err)
	require.True(t, errors.Is(err, os.ErrNotExist))
	require.Equal(t, "quarterly-report", readStoredValue(t, store, "Acme Renamed/docs/report.txt"))

	zipPath := filepath.Join(t.TempDir(), "acme.zip")
	require.NoError(t, store.ZipFolder("Acme Renamed", zipPath))
	zipReader, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer zipReader.Close()
	require.Len(t, zipReader.File, 1)
	require.Equal(t, "docs/report.txt", zipReader.File[0].Name)
	zippedFile, err := zipReader.File[0].Open()
	require.NoError(t, err)
	defer zippedFile.Close()
	zippedData, err := io.ReadAll(zippedFile)
	require.NoError(t, err)
	require.Equal(t, "quarterly-report", string(zippedData))

	require.NoError(t, store.Delete("Acme Renamed"))
	_, err = store.Download("Acme Renamed/docs/report.txt")
	require.Error(t, err)
	require.True(t, errors.Is(err, os.ErrNotExist))

	require.NoError(t, store.CreateFolder("Archive"))
	require.NoError(t, store.Delete("Archive"))
	entries, err = store.List("")
	require.NoError(t, err)
	require.Empty(t, entries)
}

func newFakeS3Storage(t *testing.T) storage.Storage {
	t.Helper()

	server := httptest.NewServer(gofakes3.New(s3mem.New()).Server())
	t.Cleanup(server.Close)

	ctx := context.Background()
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	require.NoError(t, err)

	client := awss3.NewFromConfig(awsCfg, func(options *awss3.Options) {
		options.BaseEndpoint = aws.String(server.URL)
		options.UsePathStyle = true
	})

	_, err = client.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: aws.String("clientshare-test")})
	require.NoError(t, err)

	store, err := storage.NewS3Storage(ctx, storage.S3Config{
		Bucket:       "clientshare-test",
		Region:       "us-east-1",
		Endpoint:     server.URL,
		AccessKey:    "test",
		SecretKey:    "test",
		UsePathStyle: true,
	})
	require.NoError(t, err)

	return store
}

func readStoredValue(t *testing.T, store storage.Storage, path string) string {
	t.Helper()

	reader, err := store.Download(path)
	require.NoError(t, err)
	defer reader.Close()

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	return string(data)
}
