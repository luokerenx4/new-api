package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEnsureUserExternalAccountIDSQLiteAddsNullableUniqueColumn(t *testing.T) {
	oldDB := DB
	oldLOGDB := LOG_DB
	oldMainType := common.MainDatabaseType()
	oldLogType := common.LogDatabaseType()
	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLOGDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
	})

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = database
	LOG_DB = database
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)

	require.NoError(t, database.Exec("CREATE TABLE users (id integer PRIMARY KEY, username varchar(20))").Error)
	require.NoError(t, database.Exec("INSERT INTO users (id, username) VALUES (1, 'alice'), (2, 'bob')").Error)

	require.NoError(t, ensureUserExternalAccountIDSQLite())
	require.True(t, database.Migrator().HasColumn(&User{}, "external_account_id"))

	require.NoError(t, database.Exec("UPDATE users SET external_account_id = 'acct_1' WHERE id = 1").Error)
	err = database.Exec("UPDATE users SET external_account_id = 'acct_1' WHERE id = 2").Error
	require.Error(t, err)

	require.NoError(t, ensureUserExternalAccountIDSQLite())
}

func TestEnsureTokenManagedKeyIDIndexIsNullableAndUnique(t *testing.T) {
	oldDB := DB
	oldLOGDB := LOG_DB
	oldMainType := common.MainDatabaseType()
	oldLogType := common.LogDatabaseType()
	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLOGDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
	})

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = database
	LOG_DB = database
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)

	require.NoError(t, database.Exec("CREATE TABLE tokens (id integer PRIMARY KEY, managed_key_id varchar(160))").Error)
	require.NoError(t, ensureTokenManagedKeyIDIndex())
	require.True(t, database.Migrator().HasIndex(&Token{}, "idx_tokens_managed_key_id"))

	require.NoError(t, database.Exec("INSERT INTO tokens (id, managed_key_id) VALUES (1, NULL), (2, NULL), (3, 'acct:desktop:1')").Error)
	require.Error(t, database.Exec("INSERT INTO tokens (id, managed_key_id) VALUES (4, 'acct:desktop:1')").Error)
	require.NoError(t, ensureTokenManagedKeyIDIndex())
}
