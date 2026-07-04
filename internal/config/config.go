// Package config 加载分层配置：默认值、配置文件和环境变量。
//
// 优先级（低 -> 高）:
//  1. 内置默认值
//  2. 工作目录中的 config.yaml（嵌套格式）
//  3. config/*.yaml 分片文件（按模块）
//  4. 以 MEDIASTATION_ 为前缀的环境变量
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// EnvPrefix 是所有环境变量驱动的覆盖使用的前缀。
const EnvPrefix = "MEDIASTATION"

// Load 从默认值 / 文件 / 环境读取配置。
//
// 即使没有文件也始终返回可用的 Config。
func Load() (*Config, error) {
	v := viper.New()
	setDefaults(v)

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !asConfigFileNotFound(err, &notFound) {
			return nil, fmt.Errorf("read main config: %w", err)
		}
	}

	// 合并 ./config/*.yaml 下的分片文件。
	if entries, err := os.ReadDir("config"); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}
			s := viper.New()
			s.SetConfigFile(filepath.Join("config", e.Name()))
			if err := s.ReadInConfig(); err == nil {
				_ = v.MergeConfigMap(s.AllSettings())
			}
		}
	}

	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// asConfigFileNotFound 是 errors.As 的小辅助函数，避免在这个短文件中导入 errors。
func asConfigFileNotFound(err error, target *viper.ConfigFileNotFoundError) bool {
	if err == nil {
		return false
	}
	if v, ok := err.(viper.ConfigFileNotFoundError); ok {
		*target = v
		return true
	}
	return false
}
