package yaml

import (
	"os"
	"path/filepath"

	"git.tatikoma.dev/corpix/atlas/errors"
)

const FileTag = "!file"

func FileHandler(baseDir string) TagHandler {
	return func(node *Node) error {
		path := node.Value
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return errors.Wrapf(err, "failed to read %s %q", FileTag, path)
		}
		node.SetString(string(content))
		return nil
	}
}

func WithFile(baseDir string) Option {
	return WithHandler(FileTag, FileHandler(baseDir))
}
