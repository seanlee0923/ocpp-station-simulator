package db

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open connects to the configured database and migrates the schema. driver
// is "sqlite" (default) or "mysql"; dsn is a MySQL DSN when driver is
// "mysql", or a SQLite file path (parent directories are created as needed)
// when driver is "sqlite".
func Open(driver, dsn string) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch driver {
	case "", "sqlite":
		if err := os.MkdirAll(filepath.Dir(dsn), 0o755); err != nil {
			return nil, fmt.Errorf("create sqlite data directory: %w", err)
		}
		dialector = sqlite.Open(dsn)
	case "mysql":
		dialector = mysql.Open(dsn)
	default:
		return nil, fmt.Errorf("unsupported DB_DRIVER %q (want sqlite or mysql)", driver)
	}

	database, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := database.AutoMigrate(&Station{}, &StationEvent{}, &User{}); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return database, nil
}
