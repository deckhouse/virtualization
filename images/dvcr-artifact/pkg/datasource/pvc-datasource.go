package datasource

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type PvcDataSource struct {
	directory string
	tarFile   *os.File
	tarReader *tar.Reader
}

func NewPvcDataSource(directory string) (*PvcDataSource, error) {
	tarFilePath := filepath.Join(os.TempDir(), "data.tar")
	tarFile, err := os.Create(tarFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create tar file: %w", err)
	}

	if err := createTarFromDirectory(directory, tarFile); err != nil {
		return nil, fmt.Errorf("failed to create tar archive: %w", err)
	}

	if _, err := tarFile.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek start of tar file: %w", err)
	}

	tarReader := tar.NewReader(tarFile)

	return &PvcDataSource{
		directory: directory,
		tarFile:   tarFile,
		tarReader: tarReader,
	}, nil
}

func (t *PvcDataSource) Filename() (string, error) {
	header, err := t.tarReader.Next()
	if err != nil {
		return "", err
	}
	path := strings.Split(header.Name, "/")
	return path[len(path)-1], nil
}

func (t *PvcDataSource) Length() (int, error) {
	stat, err := t.tarFile.Stat()
	if err != nil {
		return 0, fmt.Errorf("failed to stat tar file: %w", err)
	}
	return int(stat.Size()), nil
}

func (t *PvcDataSource) ReadCloser() (io.ReadCloser, error) {
	if _, err := t.tarFile.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek start of tar file: %w", err)
	}
	t.tarReader = tar.NewReader(t.tarFile)
	return t, nil
}

func (t *PvcDataSource) Read(p []byte) (n int, err error) {
	return t.tarReader.Read(p)
}

func (t *PvcDataSource) Close() error {
	return t.tarFile.Close()
}

func createTarFromDirectory(directory string, tarFile io.Writer) error {
	tarWriter := tar.NewWriter(tarFile)
	defer tarWriter.Close()

	err := filepath.Walk(directory, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if fi.Mode().IsRegular() {
			fileHeader, _ := tar.FileInfoHeader(fi, "")
			fileHeader.Name = strings.TrimPrefix(file, directory+"/")

			if err := tarWriter.WriteHeader(fileHeader); err != nil {
				return fmt.Errorf("failed to write tar header: %w", err)
			}

			fileReader, err := os.Open(file)
			if err != nil {
				return fmt.Errorf("failed to open file: %w", err)
			}
			defer fileReader.Close()

			if _, err := io.Copy(tarWriter, fileReader); err != nil {
				return fmt.Errorf("failed to write file to tar: %w", err)
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	return nil
}
