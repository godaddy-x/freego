package yaml

import (
	"errors"

	DIC "github.com/godaddy-x/freego/core/const"
	utils "github.com/godaddy-x/freego/core/str"
)

var defaultAllYamlConfig *DIC.YamlConfig

func InitAllConfig(path string) (err error) {
	defaultAllYamlConfig, err = utils.LoadYamlConfigFromPath(path)
	if err != nil {
		return err
	}
	return nil
}

func GetAllConfig() *DIC.YamlConfig {
	if defaultAllYamlConfig == nil || !defaultAllYamlConfig.CheckReady() {
		panic(errors.New("yaml config not ready"))
	}
	return defaultAllYamlConfig
}
