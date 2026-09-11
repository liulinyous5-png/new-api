package model

import (
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestChannelCredentialDatabaseRoundTrip(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(":memory:")
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN not configured")
				}
				driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			oldDB, oldMain, oldLog, oldCache := DB, common.MainDatabaseType(), common.LogDatabaseType(), common.MemoryCacheEnabled
			DB = db
			common.SetDatabaseTypes(common.DatabaseType(dialect), oldLog)
			common.MemoryCacheEnabled = false
			initCol()
			t.Cleanup(func() {
				DB = oldDB
				common.SetDatabaseTypes(oldMain, oldLog)
				common.MemoryCacheEnabled = oldCache
				initCol()
			})
			require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
			channel := Channel{Name: "account-credential-roundtrip", Type: constant.ChannelTypeAzure, Key: "legacy-one\nlegacy-two", Status: common.ChannelStatusEnabled, Models: "gpt-4o", Group: "default", ChannelInfo: ChannelInfo{IsMultiKey: true, MultiKeyMode: constant.MultiKeyModePolling}}
			require.NoError(t, db.Create(&channel).Error)
			t.Cleanup(func() {
				require.NoError(t, db.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error)
				require.NoError(t, db.Delete(&Channel{}, channel.Id).Error)
			})
			// Upgrade an existing plaintext key list, preserving a legacy key and its status.
			channel.Key = "legacy-one\n{\"key\":\"same\",\"base_url\":\"https://a.example\"}\n{\"key\":\"same\",\"base_url\":\"https://b.example\"}"
			fallback := "https://fallback.example"
			channel.BaseURL = &fallback
			channel.ChannelInfo.AccountCredentials = true
			require.NoError(t, channel.NormalizeAccountCredentials())
			channel.ChannelInfo.MultiKeyStatusList = map[int]int{0: common.ChannelStatusManuallyDisabled}
			require.NoError(t, channel.Update())
			for _, index := range []int{1, 2, 1} {
				loaded, err := GetChannelById(channel.Id, true)
				require.NoError(t, err)
				assert.Equal(t, channel.Key, loaded.Key)
				assert.True(t, loaded.ChannelInfo.AccountCredentials)
				assert.Equal(t, 3, loaded.ChannelInfo.MultiKeySize)
				entry, actual, apiErr := loaded.GetNextEnabledKey()
				require.Nil(t, apiErr)
				assert.Equal(t, index, actual)
				credential, err := loaded.ResolveCredential(entry)
				require.NoError(t, err)
				assert.Equal(t, "same", credential.Key)
				expectedURL := "https://a.example"
				if index == 2 {
					expectedURL = "https://b.example"
				}
				assert.Equal(t, expectedURL, credential.BaseURL)
			}
			var version string
			query := "SELECT version()"
			if dialect == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("verified %s %s", dialect, version)
		})
	}
}
