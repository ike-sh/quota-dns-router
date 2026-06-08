package master

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func BackupDatabase(srcPath, destPath string) (string, error) {
	srcPath = filepath.Clean(srcPath)
	if _, err := os.Stat(srcPath); err != nil {
		return "", fmt.Errorf("数据库不存在：%s", srcPath)
	}
	if destPath == "" {
		destPath = filepath.Join(filepath.Dir(srcPath), "backups")
	}
	info, err := os.Stat(destPath)
	if err == nil && info.IsDir() {
		destPath = filepath.Join(destPath, fmt.Sprintf("master-%s.db", time.Now().UTC().Format("20060102-150405")))
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", err
	}
	if err := copyFile(srcPath, destPath); err != nil {
		return "", err
	}
	return destPath, nil
}

func RestoreDatabase(srcPath, destPath string) error {
	srcPath = filepath.Clean(srcPath)
	destPath = filepath.Clean(destPath)
	if _, err := os.Stat(srcPath); err != nil {
		return fmt.Errorf("备份文件不存在：%s", srcPath)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(destPath); err == nil {
		backup := destPath + ".before-restore-" + time.Now().UTC().Format("20060102-150405")
		if err := copyFile(destPath, backup); err != nil {
			return fmt.Errorf("无法备份当前数据库：%w", err)
		}
	}
	return copyFile(srcPath, destPath)
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
