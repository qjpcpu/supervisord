package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/BurntSushi/toml"
)

var provider atomic.Value

const lock_suffix = ".lock"

type IProvider interface {
	GetConfig() *SupervisorConfig
	ReloadConfig() (*SupervisorConfig, error)
	UpdateConfig(*SupervisorConfig) error
	CheckConfigFile() error
	Close() error
}

type providerImpl struct {
	p IProvider
}

func Provider() IProvider {
	return provider.Load().(*providerImpl).p
}

func UseProvider(p IProvider) IProvider {
	provider.Store(&providerImpl{p: p})
	return p
}

func init() {
	UseProvider(NewProvider(false))
}

func NewProvider(isMaster bool) IProvider {
	p := &defaultProvider{masterMode: isMaster}
	p.configContainer.Store(&SupervisorConfigInfo{Config: &SupervisorConfig{}})
	p.LoadConfig(true)
	return p
}

type defaultProvider struct {
	masterMode      bool
	configContainer atomic.Value
}

func (self *defaultProvider) GetConfig() *SupervisorConfig {
	s := self.configContainer.Load().(*SupervisorConfigInfo)
	return s.Config
}

func (self *defaultProvider) UpdateConfig(c *SupervisorConfig) error {
	oldinfo := self.configContainer.Load().(*SupervisorConfigInfo)
	info := &SupervisorConfigInfo{
		Config: c,
		File:   oldinfo.File,
	}
	if info.File == "" {
		info.File = filepath.Join(supervisordDir(), `../conf/supervisord.conf`)
	}
	dir := filepath.Dir(info.File)
	if _, err := os.Stat(dir); err != nil {
		os.MkdirAll(dir, 0755)
	}
	bytes, err := config_marshal(c)
	if err != nil {
		return err
	}
	if err := os.WriteFile(info.File, bytes, 0644); err != nil {
		return err
	}
	self.configContainer.Store(info)
	return self.syncConfigLock(c, info.File)
}

func (self *defaultProvider) LoadConfig(isInit bool) error {
	file, err := findSupervisordConf()
	if err != nil {
		return err
	}
	preferLock := true
	if self.masterMode && isInit {
		preferLock = false
	}
	if preferLock {
		if lock := self.lockFile(file); lock != file {
			if err := self.loadConfig(lock); err == nil {
				return nil
			}
		}
	}
	if err = self.loadConfig(file); err != nil {
		return err
	}
	return self.syncConfigLock(self.GetConfig(), file)
}

func (self *defaultProvider) ReloadConfig() (*SupervisorConfig, error) {
	file, err := findSupervisordConf()
	if err != nil {
		return nil, err
	}
	if err = self.loadConfig(file); err != nil {
		return nil, err
	}
	c := self.GetConfig()
	return c, self.syncConfigLock(c, file)
}

func (self *defaultProvider) lockFile(file string) string {
	if !self.isLockFile(file) {
		dir := filepath.Dir(file)
		file = filepath.Base(file)
		return filepath.Join(dir, "."+file+lock_suffix)
	}
	return file
}

func (self *defaultProvider) isLockFile(file string) bool {
	return strings.HasSuffix(file, lock_suffix)
}

func (self *defaultProvider) loadConfig(file string) error {
	bytes, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	cnf, err := config_unmarshal(bytes)
	if err != nil {
		return err
	}
	self.configContainer.Store(&SupervisorConfigInfo{
		Config: cnf,
		File:   file,
	})
	return nil
}

func (self *defaultProvider) syncConfigLock(config *SupervisorConfig, file string) error {
	if !self.masterMode || self.isLockFile(file) {
		return nil
	}
	lockFile := self.lockFile(file)
	return self.syncConfig(config, lockFile)
}

func (self *defaultProvider) syncConfig(config *SupervisorConfig, file string) error {
	data, err := config_marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(file, data, 0644)
}

func (self *defaultProvider) CheckConfigFile() error {
	file, err := findSupervisordConf()
	if err != nil {
		return err
	}
	bytes, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	_, err = config_unmarshal(bytes)
	return nil
}

func (self *defaultProvider) Close() error {
	if self.masterMode {
		if file := self.configContainer.Load().(*SupervisorConfigInfo).File; file != "" {
			if !self.isLockFile(file) {
				file = self.lockFile(file)
			}
			os.RemoveAll(file)
		}
	}
	return nil
}

func config_marshal(c *SupervisorConfig) ([]byte, error) {
	return toml.Marshal(c)
}

func config_unmarshal(bs []byte) (*SupervisorConfig, error) {
	var cnf SupervisorConfig
	if err := toml.Unmarshal(bs, &cnf); err != nil {
		return nil, err
	}
	return &cnf, nil
}
