package client

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wjzhangq/sshlink/internal/common"
)

// AddAuthorizedKey 将公钥添加到 authorized_keys（如果不存在）
func AddAuthorizedKey(pubKey string) error {
	authKeysPath, err := authorizedKeysPath()
	if err != nil {
		return err
	}

	// 确保 .ssh 目录存在
	sshDir := filepath.Dir(authKeysPath)
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("create .ssh dir error: %w", err)
	}

	// 检查公钥是否已存在
	exists, err := keyExists(authKeysPath, pubKey)
	if err != nil {
		return err
	}
	if exists {
		common.Debug("public key already in authorized_keys")
		return nil
	}

	// 追加公钥
	f, err := os.OpenFile(authKeysPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open authorized_keys error: %w", err)
	}
	defer f.Close()

	line := strings.TrimSpace(pubKey) + "\n"
	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("write authorized_keys error: %w", err)
	}

	// 修复权限
	if err := fixPermissions(authKeysPath); err != nil {
		common.Error("fix authorized_keys permissions error: %v", err)
	}

	common.Info("added public key to %s", authKeysPath)
	return nil
}

func authorizedKeysPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir error: %w", err)
	}
	return filepath.Join(homeDir, ".ssh", "authorized_keys"), nil
}

func keyExists(path, pubKey string) (bool, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()

	// 提取公钥的关键部分（类型 + 内容，忽略注释）
	keyParts := strings.Fields(pubKey)
	if len(keyParts) < 2 {
		return false, nil
	}
	keyID := keyParts[0] + " " + keyParts[1]

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, keyID) {
			return true, nil
		}
	}
	return false, scanner.Err()
}

func fixPermissions(path string) error {
	if runtime.GOOS == "windows" {
		// Windows 不需要 chmod，确保文件可读即可
		return nil
	}
	return os.Chmod(path, 0600)
}
