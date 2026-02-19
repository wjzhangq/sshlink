package server

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wjzhangq/sshlink/internal/common"
)

// ConfigManager SSH配置管理器
type ConfigManager struct {
	mu         sync.Mutex
	configPath string
}

// HostConfig SSH Host配置
type HostConfig struct {
	Host         string
	HostName     string
	User         string
	Port         int
	IdentityFile string
	Comment      string // 注释信息
}

// NewConfigManager 创建配置管理器
func NewConfigManager() (*ConfigManager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir error: %w", err)
	}

	configPath := filepath.Join(homeDir, ".ssh", "config")

	return &ConfigManager{
		configPath: configPath,
	}, nil
}

// Backup 备份配置文件
func (cm *ConfigManager) Backup() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 检查配置文件是否存在
	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		// 配置文件不存在，创建空文件
		sshDir := filepath.Dir(cm.configPath)
		if err := os.MkdirAll(sshDir, 0700); err != nil {
			return fmt.Errorf("create .ssh dir error: %w", err)
		}

		f, err := os.Create(cm.configPath)
		if err != nil {
			return fmt.Errorf("create config file error: %w", err)
		}
		f.Close()

		return nil
	}

	// 备份现有配置
	backupPath := fmt.Sprintf("%s.bak.%s", cm.configPath, time.Now().Format("20060102150405"))
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		return fmt.Errorf("read config error: %w", err)
	}

	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return fmt.Errorf("write backup error: %w", err)
	}

	common.Info("backed up SSH config to %s", backupPath)
	return nil
}

// AddHost 添加Host配置
func (cm *ConfigManager) AddHost(config HostConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 读取现有配置
	hosts, otherLines, err := cm.readConfig()
	if err != nil {
		return err
	}

	// 检查是否已存在
	for i, h := range hosts {
		if h.Host == config.Host {
			// 更新现有配置
			hosts[i] = config
			if err := cm.writeConfig(hosts, otherLines); err != nil {
				return err
			}
			common.Info("SSH config updated: host %s", config.Host)
			return nil
		}
	}

	// 添加新配置
	hosts = append(hosts, config)
	if err := cm.writeConfig(hosts, otherLines); err != nil {
		return err
	}
	common.Info("SSH config added: host %s", config.Host)
	return nil
}

// RemoveAll 移除所有 sshlink Host 配置
func (cm *ConfigManager) RemoveAll() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	_, otherLines, err := cm.readConfig()
	if err != nil {
		return err
	}

	if err := cm.writeConfig(nil, otherLines); err != nil {
		return err
	}
	common.Info("SSH config: all sshlink hosts removed")
	return nil
}

// RemoveHost 移除Host配置
func (cm *ConfigManager) RemoveHost(host string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 读取现有配置
	hosts, otherLines, err := cm.readConfig()
	if err != nil {
		return err
	}

	// 查找并移除
	newHosts := make([]HostConfig, 0, len(hosts))
	for _, h := range hosts {
		if h.Host != host {
			newHosts = append(newHosts, h)
		}
	}

	if err := cm.writeConfig(newHosts, otherLines); err != nil {
		return err
	}
	common.Info("SSH config removed: host %s", host)
	return nil
}

// readConfig 读取配置文件
func (cm *ConfigManager) readConfig() ([]HostConfig, []string, error) {
	file, err := os.Open(cm.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	defer file.Close()

	var hosts []HostConfig
	var otherLines []string
	var currentHost *HostConfig
	var currentComment string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// 检查是否是 sshlink 的 Host
		if strings.HasPrefix(trimmed, "Host ") {
			parts := strings.SplitN(trimmed, "#", 2)
			hostLine := strings.TrimSpace(parts[0])
			hostName := strings.TrimSpace(strings.TrimPrefix(hostLine, "Host "))

			if strings.HasPrefix(hostName, "sshlink") {
				// 保存上一个 Host
				if currentHost != nil {
					hosts = append(hosts, *currentHost)
				}

				// 开始新的 sshlink Host
				currentHost = &HostConfig{
					Host: hostName,
				}

				// 提取注释
				if len(parts) > 1 {
					currentComment = strings.TrimSpace(parts[1])
					currentHost.Comment = currentComment
				}
			} else {
				// 非 sshlink Host，保存到 otherLines
				if currentHost != nil {
					hosts = append(hosts, *currentHost)
					currentHost = nil
				}
				otherLines = append(otherLines, line)
			}
		} else if currentHost != nil {
			// 解析 sshlink Host 的配置项
			if strings.HasPrefix(trimmed, "HostName ") {
				currentHost.HostName = strings.TrimSpace(strings.TrimPrefix(trimmed, "HostName "))
			} else if strings.HasPrefix(trimmed, "User ") {
				currentHost.User = strings.TrimSpace(strings.TrimPrefix(trimmed, "User "))
			} else if strings.HasPrefix(trimmed, "Port ") {
				fmt.Sscanf(trimmed, "Port %d", &currentHost.Port)
			} else if strings.HasPrefix(trimmed, "IdentityFile ") {
				currentHost.IdentityFile = strings.TrimSpace(strings.TrimPrefix(trimmed, "IdentityFile "))
			}
		} else {
			// 非 sshlink 配置
			otherLines = append(otherLines, line)
		}
	}

	// 保存最后一个 Host
	if currentHost != nil {
		hosts = append(hosts, *currentHost)
	}

	return hosts, otherLines, scanner.Err()
}

// writeConfig 写入配置文件（原子操作）
func (cm *ConfigManager) writeConfig(hosts []HostConfig, otherLines []string) error {
	// 写入临时文件
	tmpFile := cm.configPath + ".tmp"
	f, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("create temp file error: %w", err)
	}

	writer := bufio.NewWriter(f)

	// 写入非 sshlink 配置
	for _, line := range otherLines {
		fmt.Fprintln(writer, line)
	}

	// 写入 sshlink 配置
	for _, host := range hosts {
		if host.Comment != "" {
			fmt.Fprintf(writer, "Host %s  # %s\n", host.Host, host.Comment)
		} else {
			fmt.Fprintf(writer, "Host %s\n", host.Host)
		}

		if host.HostName != "" {
			fmt.Fprintf(writer, "    HostName %s\n", host.HostName)
		}
		if host.User != "" {
			fmt.Fprintf(writer, "    User %s\n", host.User)
		}
		if host.Port > 0 {
			fmt.Fprintf(writer, "    Port %d\n", host.Port)
		}
		if host.IdentityFile != "" {
			fmt.Fprintf(writer, "    IdentityFile %s\n", host.IdentityFile)
		}
		fmt.Fprintln(writer, "    StrictHostKeyChecking accept-new")
		fmt.Fprintln(writer)
	}

	if err := writer.Flush(); err != nil {
		f.Close()
		os.Remove(tmpFile)
		return fmt.Errorf("flush error: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("close temp file error: %w", err)
	}

	// 原子重命名
	if err := os.Rename(tmpFile, cm.configPath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("rename error: %w", err)
	}

	// 设置权限
	if err := os.Chmod(cm.configPath, 0600); err != nil {
		return fmt.Errorf("chmod error: %w", err)
	}

	return nil
}
