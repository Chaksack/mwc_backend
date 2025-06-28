package config

import (
	"fmt"
	"log" // Added log for warnings
	"os"
	"strconv" // Added for SMTPPort parsing
	"strings" // Added for splitting supported languages

	"github.com/spf13/viper"
)

// Config holds all configuration for the application
type Config struct {
	DatabaseURL  string `mapstructure:"DATABASE_URL"`
	RabbitMQURL  string `mapstructure:"RABBITMQ_URL"`
	JWTSecret    string `mapstructure:"JWT_SECRET"`
	SMTPHost     string `mapstructure:"SMTP_HOST"`
	SMTPPort     int    `mapstructure:"SMTP_PORT"`
	SMTPUser     string `mapstructure:"SMTP_USER"`
	SMTPPassword string `mapstructure:"SMTP_PASSWORD"`
	EmailFrom    string `mapstructure:"EMAIL_FROM"`
	// Add other configurations as needed, e.g., JWT_EXPIRATION_HOURS
	JwtExpirationHours int `mapstructure:"JWT_EXPIRATION_HOURS"`
	// Stripe configuration
	StripeSecretKey      string `mapstructure:"STRIPE_SECRET_KEY"`
	StripePublishableKey string `mapstructure:"STRIPE_PUBLISHABLE_KEY"`
	StripeWebhookSecret  string `mapstructure:"STRIPE_WEBHOOK_SECRET"`
	// Subscription prices
	StripeMonthlyPriceID string `mapstructure:"STRIPE_MONTHLY_PRICE_ID"`
	StripeAnnualPriceID  string `mapstructure:"STRIPE_ANNUAL_PRICE_ID"`
	// WebSocket configuration
	WebSocketEnabled bool   `mapstructure:"WEBSOCKET_ENABLED"`
	WebSocketPath    string `mapstructure:"WEBSOCKET_PATH"`
	// I18n configuration
	DefaultLanguage string   `mapstructure:"DEFAULT_LANGUAGE"`
	SupportedLanguages []string `mapstructure:"SUPPORTED_LANGUAGES"`
	// Default admin user configuration
	DefaultAdminEmail     string `mapstructure:"DEFAULT_ADMIN_EMAIL"`
	DefaultAdminPassword  string `mapstructure:"DEFAULT_ADMIN_PASSWORD"`
	DefaultAdminFirstName string `mapstructure:"DEFAULT_ADMIN_FIRST_NAME"`
	DefaultAdminLastName  string `mapstructure:"DEFAULT_ADMIN_LAST_NAME"`
}

// LoadConfig reads configuration from file or environment variables.
func LoadConfig() (*Config, error) {
	viper.AddConfigPath(".")
	viper.SetConfigName(".env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("unable to decode into struct: %w", err)
	}

	// Fallback and validation for critical configurations
	if config.DatabaseURL == "" {
		config.DatabaseURL = os.Getenv("DATABASE_URL")
		if config.DatabaseURL == "" {
			return nil, fmt.Errorf("DATABASE_URL is not set")
		}
	}
	if config.RabbitMQURL == "" {
		config.RabbitMQURL = os.Getenv("RABBITMQ_URL")
		if config.RabbitMQURL == "" {
			return nil, fmt.Errorf("RABBITMQ_URL is not set")
		}
	}
	if config.JWTSecret == "" {
		config.JWTSecret = os.Getenv("JWT_SECRET")
		if config.JWTSecret == "" {
			return nil, fmt.Errorf("JWT_SECRET is not set")
		}
	}

	// SMTP Configuration with fallbacks and logging
	if config.SMTPHost == "" {
		config.SMTPHost = os.Getenv("SMTP_HOST")
	}
	if config.SMTPPort == 0 {
		portStr := os.Getenv("SMTP_PORT")
		if portStr != "" {
			parsedPort, err := strconv.Atoi(portStr)
			if err == nil {
				config.SMTPPort = parsedPort
			} else {
				log.Printf("Warning: Invalid SMTP_PORT value '%s'. Using default or 0.", portStr)
			}
		}
	}
	if config.SMTPUser == "" {
		config.SMTPUser = os.Getenv("SMTP_USER")
	}
	if config.SMTPPassword == "" {
		config.SMTPPassword = os.Getenv("SMTP_PASSWORD")
	}
	if config.EmailFrom == "" {
		config.EmailFrom = os.Getenv("EMAIL_FROM")
	}

	if config.SMTPHost == "" || config.SMTPPort == 0 || config.EmailFrom == "" {
		log.Println("Warning: SMTP configuration is not fully set. Email functionality might be limited or disabled.")
	}

	// JWT Expiration
	if config.JwtExpirationHours == 0 {
		expHoursStr := os.Getenv("JWT_EXPIRATION_HOURS")
		if expHoursStr != "" {
			parsedHours, err := strconv.Atoi(expHoursStr)
			if err == nil && parsedHours > 0 {
				config.JwtExpirationHours = parsedHours
			} else {
				config.JwtExpirationHours = 72 // Default to 72 hours
				log.Printf("Warning: Invalid or missing JWT_EXPIRATION_HOURS. Defaulting to %d hours.", config.JwtExpirationHours)
			}
		} else {
			config.JwtExpirationHours = 72 // Default to 72 hours
			log.Printf("Warning: JWT_EXPIRATION_HOURS not set. Defaulting to %d hours.", config.JwtExpirationHours)
		}
	}

	// Stripe Configuration
	if config.StripeSecretKey == "" {
		config.StripeSecretKey = os.Getenv("STRIPE_SECRET_KEY")
		if config.StripeSecretKey == "" {
			log.Println("Warning: STRIPE_SECRET_KEY is not set. Payment functionality will be disabled.")
		}
	}
	if config.StripePublishableKey == "" {
		config.StripePublishableKey = os.Getenv("STRIPE_PUBLISHABLE_KEY")
		if config.StripePublishableKey == "" {
			log.Println("Warning: STRIPE_PUBLISHABLE_KEY is not set. Payment functionality will be disabled.")
		}
	}
	if config.StripeWebhookSecret == "" {
		config.StripeWebhookSecret = os.Getenv("STRIPE_WEBHOOK_SECRET")
		if config.StripeWebhookSecret == "" {
			log.Println("Warning: STRIPE_WEBHOOK_SECRET is not set. Stripe webhook verification will be disabled.")
		}
	}
	if config.StripeMonthlyPriceID == "" {
		config.StripeMonthlyPriceID = os.Getenv("STRIPE_MONTHLY_PRICE_ID")
		if config.StripeMonthlyPriceID == "" {
			log.Println("Warning: STRIPE_MONTHLY_PRICE_ID is not set. Monthly subscription plan will be unavailable.")
		}
	}
	if config.StripeAnnualPriceID == "" {
		config.StripeAnnualPriceID = os.Getenv("STRIPE_ANNUAL_PRICE_ID")
		if config.StripeAnnualPriceID == "" {
			log.Println("Warning: STRIPE_ANNUAL_PRICE_ID is not set. Annual subscription plan will be unavailable.")
		}
	}

	// WebSocket Configuration
	webSocketEnabledStr := os.Getenv("WEBSOCKET_ENABLED")
	if webSocketEnabledStr != "" {
		config.WebSocketEnabled = webSocketEnabledStr == "true" || webSocketEnabledStr == "1"
	} else {
		config.WebSocketEnabled = false // Default to disabled
	}
	if config.WebSocketPath == "" {
		config.WebSocketPath = os.Getenv("WEBSOCKET_PATH")
		if config.WebSocketPath == "" {
			config.WebSocketPath = "/ws" // Default WebSocket path
		}
	}

	// I18n Configuration
	if config.DefaultLanguage == "" {
		config.DefaultLanguage = os.Getenv("DEFAULT_LANGUAGE")
		if config.DefaultLanguage == "" {
			config.DefaultLanguage = "en" // Default to English
		}
	}
	supportedLangsStr := os.Getenv("SUPPORTED_LANGUAGES")
	if supportedLangsStr != "" {
		config.SupportedLanguages = strings.Split(supportedLangsStr, ",")
	} else if len(config.SupportedLanguages) == 0 {
		config.SupportedLanguages = []string{"en"} // Default to English only
	}

	// Default Admin User Configuration
	if config.DefaultAdminEmail == "" {
		config.DefaultAdminEmail = os.Getenv("DEFAULT_ADMIN_EMAIL")
		if config.DefaultAdminEmail == "" {
			config.DefaultAdminEmail = "admin@example.com" // Default admin email
			log.Println("Warning: DEFAULT_ADMIN_EMAIL not set. Using default value:", config.DefaultAdminEmail)
		}
	}
	if config.DefaultAdminPassword == "" {
		config.DefaultAdminPassword = os.Getenv("DEFAULT_ADMIN_PASSWORD")
		if config.DefaultAdminPassword == "" {
			config.DefaultAdminPassword = "Admin123!" // Default admin password
			log.Println("Warning: DEFAULT_ADMIN_PASSWORD not set. Using default value. Please change this in production!")
		}
	}
	if config.DefaultAdminFirstName == "" {
		config.DefaultAdminFirstName = os.Getenv("DEFAULT_ADMIN_FIRST_NAME")
		if config.DefaultAdminFirstName == "" {
			config.DefaultAdminFirstName = "Admin" // Default admin first name
		}
	}
	if config.DefaultAdminLastName == "" {
		config.DefaultAdminLastName = os.Getenv("DEFAULT_ADMIN_LAST_NAME")
		if config.DefaultAdminLastName == "" {
			config.DefaultAdminLastName = "User" // Default admin last name
		}
	}

	return &config, nil
}
