package helper

import (
	"code_nim/log"
	"code_nim/model"
	"gopkg.in/yaml.v3"
	"os"
)

func LoadConfigFile(cfg *model.Task) {
	f, err := os.ReadFile("config_file/review-config.yaml")
	if err != nil {
		log.Error(err)
	}

	err = yaml.Unmarshal(f, &cfg)
	if err != nil {
		log.Error(err)
	}
}
