package decrypt

import (
	"path/filepath"

	"github.com/rs/zerolog/log"

	"github.com/sjzar/chatlog/internal/wechat/decrypt/common"
	"github.com/sjzar/chatlog/pkg/util/dat2img"
)

type Validator struct {
	platform        string
	version         int
	dbPath          string
	decryptor       Decryptor
	dbFile          *common.DBFile
	imgKeyValidator *dat2img.AesKeyValidator
}

// NewValidator 创建一个仅用于验证的验证器
func NewValidator(platform string, version int, dataDir string) (*Validator, error) {
	return NewValidatorWithFile(platform, version, dataDir)
}

func NewValidatorWithFile(platform string, version int, dataDir string) (*Validator, error) {
	log.Debug().Str("platform", platform).Int("version", version).Str("dataDir", dataDir).Msg("creating validator with file")
	dbFile := GetSimpleDBFile(platform, version)
	dbPath := filepath.Join(dataDir, dbFile)
	decryptor, err := NewDecryptor(platform, version)
	if err != nil {
		log.Debug().Err(err).Msg("failed to create decryptor for validator")
		return nil, err
	}
	d, err := common.OpenDBFile(dbPath, decryptor.GetPageSize())
	if err != nil {
		log.Debug().Err(err).Str("path", dbPath).Msg("failed to open db file for validator")
		return nil, err
	}

	validator := &Validator{
		platform:  platform,
		version:   version,
		dbPath:    dbPath,
		decryptor: decryptor,
		dbFile:    d,
	}

	if version == 4 {
		validator.imgKeyValidator = dat2img.NewImgKeyValidator(dataDir)
	}

	return validator, nil
}

func (v *Validator) Validate(key []byte) bool {
	valid := v.decryptor.Validate(v.dbFile.FirstPage, key)
	if valid {
		log.Debug().Msg("db key validation passed")
	}
	return valid
}

func (v *Validator) ValidateImgKey(key []byte) bool {
	if v.imgKeyValidator == nil {
		log.Debug().Msg("img key validator is nil")
		return false
	}
	valid := v.imgKeyValidator.Validate(key)
	if valid {
		log.Debug().Msg("img key validation passed")
	}
	return valid
}

func GetSimpleDBFile(platform string, version int) string {
	switch {
	case platform == "windows" && version == 3:
		return "Msg\\Misc.db"
	case platform == "windows" && version == 4:
		return "db_storage\\message\\message_0.db"
	case platform == "darwin" && version == 3:
		return "Message/msg_0.db"
	case platform == "darwin" && version == 4:
		return "db_storage/message/message_0.db"
	}
	return ""

}
