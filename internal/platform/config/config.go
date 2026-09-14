package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"reflect"
	"strings"

	"github.com/joho/godotenv"
)

// variaveis de ambiente ou .env
type Config struct {
	AppEnv               string `env:"BRIDOPEN_APP_ENV,development"`
	ServerPort           string `env:"BRIDOPEN_SERVER_PORT,5000"`
	BaseURL              string `env:"BRIDOPEN_BASE_URL,http://localhost:5000"`
	DBConnURL            string `env:"BRIDOPEN_DB_CONN_URL,required"`
	LevelLog             string `env:"BRIDOPEN_LEVEL_LOG,info"`
	MailHost             string `env:"BRIDOPEN_MAIL_HOST,required"`
	MailPort             string `env:"BRIDOPEN_MAIL_PORT,required"`
	MailUserName         string `env:"BRIDOPEN_MAIL_USERNAME,required"`
	MailPassword         string `env:"BRIDOPEN_MAIL_PASSWORD,required"`
	MailFrom             string `env:"BRIDOPEN_MAIL_FROM,nao-responder@bridopen.local"`
	CSRFKey              string `env:"BRIDOPEN_CSRF_KEY,required"`
	MFASecretKey         string `env:"BRIDOPEN_MFA_SECRET_KEY,"`
	CookieSecure         string `env:"BRIDOPEN_COOKIE_SECURE,false"`
	CookieDomain         string `env:"BRIDOPEN_COOKIE_DOMAIN,"`
	TrustedOrigins       string `env:"BRIDOPEN_TRUSTED_ORIGINS,"`
	AttachmentStorageDir string `env:"BRIDOPEN_ATTACHMENT_STORAGE_DIR,storage/attachments"`
	ClamAVAddr           string `env:"BRIDOPEN_CLAMAV_ADDR,"`
}

func (c Config) GetLevelLog() slog.Level {
	switch strings.ToLower(c.LevelLog) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func (c Config) SPrint() (envs string) {
	v := reflect.ValueOf(c)
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		envTag := strings.Split(field.Tag.Get("env"), ",")
		name := envTag[0]
		value := envTag[1]
		envs += fmt.Sprintf("%s - %s\n", name, value)
	}
	return
}

func (c Config) IsProduction() bool {
	env := strings.ToLower(strings.TrimSpace(c.AppEnv))
	return env == "production" || env == "prod"
}

func (c Config) UseSecureCookies() bool {
	switch strings.ToLower(strings.TrimSpace(c.CookieSecure)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func (c Config) MFAEncryptionSecret() string {
	if strings.TrimSpace(c.MFASecretKey) != "" {
		return c.MFASecretKey
	}
	if c.IsProduction() {
		return ""
	}
	return c.CSRFKey
}

func (c Config) TrustedOriginList() []string {
	origins := []string{
		fmt.Sprintf("localhost:%s", c.ServerPort),
		fmt.Sprintf("127.0.0.1:%s", c.ServerPort),
		fmt.Sprintf("192.168.1.4:%s", c.ServerPort),
	}

	if u, err := url.Parse(c.BaseURL); err == nil && u.Host != "" {
		origins = append(origins, u.Host)
	}

	for _, origin := range strings.Split(c.TrustedOrigins, ",") {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			origins = append(origins, origin)
		}
	}

	return uniqueStrings(origins)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// carregar as variaveis de ambiente
func (c Config) loadFromEnv() (conf Config) {
	v := reflect.ValueOf(c)
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		envTag := strings.Split(field.Tag.Get("env"), ",")
		envName := envTag[0]
		defaultValue := envTag[1]
		value := os.Getenv(envName)

		if value == "" && defaultValue != "required" {
			f := reflect.ValueOf(&conf).Elem().FieldByName(field.Name)
			f.SetString(defaultValue)
		} else {
			f := reflect.ValueOf(&conf).Elem().FieldByName(field.Name)
			f.SetString(value)
		}
	}
	return
}

// valida se foi preenchido as informações
func (c Config) validate() {
	var validationMsg string
	v := reflect.ValueOf(c)
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		value := v.Field(i)
		envTag := strings.Split(t.Field(i).Tag.Get("env"), ",")
		envName := envTag[0]
		envValue := envTag[1]

		if envValue == "required" && value.String() == "" {
			validationMsg += fmt.Sprintf("%s is required\n", envName)
		}
	}

	if len(validationMsg) != 0 {
		panic(validationMsg)
	}

	baseURL, err := url.Parse(c.BaseURL)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		panic("BRIDOPEN_BASE_URL deve ser uma URL absoluta, por exemplo https://app.exemplo.com")
	}

	if c.IsProduction() {
		if baseURL.Scheme != "https" {
			panic("BRIDOPEN_BASE_URL deve usar https em producao")
		}
		if !c.UseSecureCookies() {
			panic("BRIDOPEN_COOKIE_SECURE deve ser true em producao")
		}
		if len(c.CSRFKey) < 32 {
			panic("BRIDOPEN_CSRF_KEY deve ter pelo menos 32 caracteres em producao")
		}
		mfaSecret := strings.TrimSpace(c.MFASecretKey)
		if mfaSecret == "" {
			panic("BRIDOPEN_MFA_SECRET_KEY deve ser configurada em producao")
		}
		if len(mfaSecret) < 32 {
			panic("BRIDOPEN_MFA_SECRET_KEY deve ter pelo menos 32 caracteres em producao")
		}
		if mfaSecret == strings.TrimSpace(c.CSRFKey) {
			panic("BRIDOPEN_MFA_SECRET_KEY deve ser independente de BRIDOPEN_CSRF_KEY em producao")
		}
	}
}

func Load() Config {
	_ = godotenv.Load()
	config := Config{}
	config = config.loadFromEnv()
	config.validate()
	return config
}
