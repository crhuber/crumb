package backend

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Backend stores encrypted data in an S3-compatible object store.
type S3Backend struct {
	Bucket      string
	Key         string
	EndpointURL string
	client      *s3.Client
}

func (b *S3Backend) getClient() (*s3.Client, error) {
	if b.client != nil {
		return b.client, nil
	}

	var opts []func(*awsconfig.LoadOptions) error
	if b.EndpointURL != "" {
		opts = append(opts, awsconfig.WithBaseEndpoint(b.EndpointURL))
	}

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, err
	}

	b.client = s3.NewFromConfig(cfg, func(o *s3.Options) {
		if b.EndpointURL != "" {
			o.UsePathStyle = true
		}
	})
	return b.client, nil
}

func (b *S3Backend) Read() ([]byte, error) {
	client, err := b.getClient()
	if err != nil {
		return nil, err
	}

	out, err := client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(b.Bucket),
		Key:    aws.String(b.Key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, nil
		}
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

func (b *S3Backend) Write(data []byte) error {
	client, err := b.getClient()
	if err != nil {
		return err
	}

	_, err = client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(b.Bucket),
		Key:    aws.String(b.Key),
		Body:   bytes.NewReader(data),
	})
	return err
}

func (b *S3Backend) Exists() (bool, error) {
	client, err := b.getClient()
	if err != nil {
		return false, err
	}

	_, err = client.HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String(b.Bucket),
		Key:    aws.String(b.Key),
	})
	if err != nil {
		var nf *types.NotFound
		if errors.As(err, &nf) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
