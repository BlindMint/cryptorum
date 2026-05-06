package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

// Config holds the application configuration
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Auth      AuthConfig      `mapstructure:"auth"`
	Libraries []LibraryConfig `mapstructure:"libraries"`
	Bookdrop  BookdropConfig  `mapstructure:"bookdrop"`
	Metadata  MetadataConfig  `mapstructure:"metadata"`
	Email     EmailConfig     `mapstructure:"email"`
	Tasks     TasksConfig     `mapstructure:"tasks"`
	Logging   LoggingConfig   `mapstructure:"logging"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port     string `mapstructure:"port"`
	DataPath string `mapstructure:"data_path"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	Mode            string        `mapstructure:"mode"` // none, password, trusted_network
	Username        string        `mapstructure:"username"`
	PasswordHash    string        `mapstructure:"password_hash"`
	SessionDuration time.Duration `mapstructure:"session_duration"`
	TrustedNetworks []string      `mapstructure:"trusted_networks"`
}

// LibraryConfig holds library configuration
type LibraryConfig struct {
	Name  string   `mapstructure:"name"`
	Paths []string `mapstructure:"paths"`
}

// BookdropConfig holds BookDrop configuration
type BookdropConfig struct {
	Path string `mapstructure:"path"`
}

// MetadataConfig holds metadata configuration
type MetadataConfig struct {
	Providers         []string `mapstructure:"providers"`
	AutoFetchOnImport bool     `mapstructure:"auto_fetch_on_import"`
}

// EmailConfig holds email configuration
type EmailConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	SMTPHost      string `mapstructure:"smtp_host"`
	SMTPPort      int    `mapstructure:"smtp_port"`
	SMTPUser      string `mapstructure:"smtp_user"`
	SMTPPass      string `mapstructure:"smtp_pass"`
	FromAddress   string `mapstructure:"from_address"`
	KindleAddress string `mapstructure:"kindle_address"`
}

// TasksConfig holds task configuration
type TasksConfig struct {
	LibraryScan     TaskSchedule `mapstructure:"library_scan"`
	MetadataRefresh TaskSchedule `mapstructure:"metadata_refresh"`
	DatabaseBackup  BackupConfig `mapstructure:"database_backup"`
}

// TaskSchedule holds a cron schedule
type TaskSchedule struct {
	Cron string `mapstructure:"cron"`
}

// BackupConfig holds backup configuration
type BackupConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Cron     string `mapstructure:"cron"`
	KeepLast int    `mapstructure:"keep_last"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level"`  // debug, info, warn, error
	Format string `mapstructure:"format"` // json, text
}

// Load loads configuration from file
func Load(configPath string) (*Config, error) {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	// Set defaults
	viper.SetDefault("server.port", "6060")
	viper.SetDefault("server.data_path", "./data")
	viper.SetDefault("auth.mode", "none")
	viper.SetDefault("auth.session_duration", "720h")
	viper.SetDefault("bookdrop.path", "./bookdrop")
	viper.SetDefault("metadata.auto_fetch_on_import", true)
	viper.SetDefault("metadata.providers", []string{"google_books", "open_library", "bookbrainz", "library_of_congress", "wikidata", "internet_archive"})
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "json")
	viper.SetDefault("tasks.database_backup.enabled", true)
	viper.SetDefault("tasks.database_backup.keep_last", 14)

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := validate(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Ensure data directory exists
	if err := os.MkdirAll(config.Server.DataPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Ensure library paths exist
	for _, lib := range config.Libraries {
		for _, path := range lib.Paths {
			if err := os.MkdirAll(path, 0755); err != nil {
				// Just warn, don't fail - path might be read-only
				fmt.Printf("Warning: library path %s does not exist or cannot be created\n", path)
			}
		}
	}

	// Ensure bookdrop directory exists
	if config.Bookdrop.Path != "" {
		if err := os.MkdirAll(config.Bookdrop.Path, 0755); err != nil {
			fmt.Printf("Warning: bookdrop path %s cannot be created\n", config.Bookdrop.Path)
		}
	}

	return &config, nil
}

// validate validates the configuration
func validate(config *Config) error {
	// Validate auth mode
	switch config.Auth.Mode {
	case "none", "password", "trusted_network":
		// valid
	default:
		return fmt.Errorf("invalid auth mode: %s", config.Auth.Mode)
	}

	// If password mode, username and password hash must be set
	if config.Auth.Mode == "password" {
		if config.Auth.Username == "" {
			return fmt.Errorf("username required for password auth mode")
		}
		if config.Auth.PasswordHash == "" {
			return fmt.Errorf("password_hash required for password auth mode")
		}
	}

	// Validate logging level
	switch config.Logging.Level {
	case "debug", "info", "warn", "error":
		// valid
	default:
		return fmt.Errorf("invalid logging level: %s", config.Logging.Level)
	}

	return nil
}

// GetDBPath returns the path to the SQLite database file
func (c *Config) GetDBPath() string {
	return filepath.Join(c.Server.DataPath, "cryptorum.db")
}

// GetCoversPath returns the path to the covers directory
func (c *Config) GetCoversPath() string {
	return filepath.Join(c.Server.DataPath, "covers")
}

// GetThumbsPath returns the path to the thumbnails directory
func (c *Config) GetThumbsPath() string {
	return filepath.Join(c.Server.DataPath, "covers", "thumb")
}

// GetFontsPath returns the path to the fonts directory
func (c *Config) GetFontsPath() string {
	return filepath.Join(c.Server.DataPath, "fonts")
}

// GetBackgroundsPath returns the path to the backgrounds directory
func (c *Config) GetBackgroundsPath() string {
	return filepath.Join(c.Server.DataPath, "backgrounds")
}

// GetBackupsPath returns the path to the backups directory
func (c *Config) GetBackupsPath() string {
	return filepath.Join(c.Server.DataPath, "backups")
}

// GetBookCachePath returns the path to the book conversion cache directory
func (c *Config) GetBookCachePath() string {
	return filepath.Join(c.Server.DataPath, "book-cache")
}

// UpdateBookdropPath persists the configured bookdrop path to the active config file.
func UpdateBookdropPath(path string) error {
	viper.Set("bookdrop.path", path)
	return viper.WriteConfig()
}

// UpdateDatabaseBackupConfig persists the scheduled database backup settings to the active config file.
func UpdateDatabaseBackupConfig(enabled bool, cron string, keepLast int) error {
	viper.Set("tasks.database_backup.enabled", enabled)
	viper.Set("tasks.database_backup.cron", cron)
	viper.Set("tasks.database_backup.keep_last", keepLast)
	return viper.WriteConfig()
}
