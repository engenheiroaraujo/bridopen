package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfigGetLevelLog verifica o mapeamento de LevelLog para níveis slog, com fallback para Info.
func TestConfigGetLevelLog(t *testing.T) {
	t.Parallel()

	assert.Equal(t, slog.LevelDebug, Config{LevelLog: "DEBUG"}.GetLevelLog())
	assert.Equal(t, slog.LevelInfo, Config{LevelLog: "info"}.GetLevelLog())
	assert.Equal(t, slog.LevelWarn, Config{LevelLog: "warn"}.GetLevelLog())
	assert.Equal(t, slog.LevelError, Config{LevelLog: "error"}.GetLevelLog())
	assert.Equal(t, slog.LevelInfo, Config{LevelLog: "unknown"}.GetLevelLog())
}

// TestConfigSPrint verifica que SPrint lista variáveis de ambiente e marca as obrigatórias.
func TestConfigSPrint(t *testing.T) {
	t.Parallel()

	out := Config{}.SPrint()
	assert.Contains(t, out, "BRIDOPEN_APP_ENV - development")
	assert.Contains(t, out, "BRIDOPEN_DB_CONN_URL - required")
	assert.Contains(t, out, "BRIDOPEN_CSRF_KEY - required")
}

// TestConfigIsProductionAndSecureCookies verifica IsProduction e UseSecureCookies para valores típicos de configuração.
func TestConfigIsProductionAndSecureCookies(t *testing.T) {
	t.Parallel()

	assert.True(t, Config{AppEnv: "production"}.IsProduction())
	assert.True(t, Config{AppEnv: "PROD"}.IsProduction())
	assert.False(t, Config{AppEnv: "development"}.IsProduction())

	assert.True(t, Config{CookieSecure: "true"}.UseSecureCookies())
	assert.True(t, Config{CookieSecure: "1"}.UseSecureCookies())
	assert.True(t, Config{CookieSecure: "yes"}.UseSecureCookies())
	assert.True(t, Config{CookieSecure: "Y"}.UseSecureCookies())
	assert.True(t, Config{CookieSecure: "on"}.UseSecureCookies())
	assert.False(t, Config{CookieSecure: "false"}.UseSecureCookies())
}

// TestConfigMFAEncryptionSecretFallback verifica o segredo MFA dedicado e o fallback para CSRFKey só em development.
func TestConfigMFAEncryptionSecretFallback(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "mfa-secret", Config{MFASecretKey: "mfa-secret", CSRFKey: "csrf"}.MFAEncryptionSecret())
	assert.Equal(t, "csrf-key", Config{AppEnv: "development", CSRFKey: "csrf-key"}.MFAEncryptionSecret())
	assert.Equal(t, "", Config{AppEnv: "production", CSRFKey: "csrf-key"}.MFAEncryptionSecret())
}

// TestConfigTrustedOriginListDedupes verifica que TrustedOriginList inclui defaults, remove duplicatas e ignora host inválido.
func TestConfigTrustedOriginListDedupes(t *testing.T) {
	t.Parallel()

	origins := Config{
		ServerPort:     "5000",
		BaseURL:        "http://localhost:5000",
		TrustedOrigins: "app.example.com, localhost:5000, ,",
	}.TrustedOriginList()

	assert.Contains(t, origins, "localhost:5000")
	assert.Contains(t, origins, "127.0.0.1:5000")
	assert.Contains(t, origins, "192.168.1.4:5000")
	assert.Contains(t, origins, "app.example.com")

	seen := map[string]int{}
	for _, origin := range origins {
		seen[origin]++
	}
	for origin, count := range seen {
		assert.Equal(t, 1, count, origin)
	}

	// Host inválido em BaseURL é ignorado; só os defaults de porta permanecem.
	noHost := Config{ServerPort: "9", BaseURL: "://bad", TrustedOrigins: ""}.TrustedOriginList()
	assert.Contains(t, noHost, "localhost:9")
	assert.NotContains(t, noHost, "://bad")
}

// TestUniqueStrings verifica que uniqueStrings remove duplicatas preservando a ordem.
func TestUniqueStrings(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"a", "b"}, uniqueStrings([]string{"a", "a", "b"}))
}

func validDevConfig() Config {
	return Config{
		AppEnv:       "development",
		BaseURL:      "http://localhost:5000",
		DBConnURL:    "postgres://x",
		MailHost:     "localhost",
		MailPort:     "587",
		MailUserName: "u",
		MailPassword: "p",
		CSRFKey:      "dev-csrf-key",
	}
}

func validProdConfig() Config {
	return Config{
		AppEnv:       "production",
		BaseURL:      "https://app.example.com",
		DBConnURL:    "postgres://x",
		MailHost:     "localhost",
		MailPort:     "587",
		MailUserName: "u",
		MailPassword: "p",
		CSRFKey:      "01234567890123456789012345678901",
		MFASecretKey: "abcdefghijklmnopqrstuvwxyz012345",
		CookieSecure: "true",
	}
}

// TestConfigValidateRequiredFieldsPanic verifica que validate entra em pânico quando faltam campos obrigatórios.
func TestConfigValidateRequiredFieldsPanic(t *testing.T) {
	t.Parallel()
	require.Panics(t, func() { Config{BaseURL: "http://localhost:5000"}.validate() })
}

// TestConfigValidateInvalidBaseURLPanic verifica que validate entra em pânico para BaseURL inválida.
func TestConfigValidateInvalidBaseURLPanic(t *testing.T) {
	t.Parallel()
	cfg := validDevConfig()
	cfg.BaseURL = "not-a-url"
	require.Panics(t, func() { cfg.validate() })

	cfg = validDevConfig()
	cfg.BaseURL = "http://"
	require.Panics(t, func() { cfg.validate() })
}

// TestConfigValidatePanicsInProduction verifica regras de segurança de produção que fazem validate entrar em pânico.
func TestConfigValidatePanicsInProduction(t *testing.T) {
	t.Parallel()

	t.Run("http base url", func(t *testing.T) {
		t.Parallel()
		cfg := validProdConfig()
		cfg.BaseURL = "http://example.com"
		require.Panics(t, func() { cfg.validate() })
	})

	t.Run("insecure cookies", func(t *testing.T) {
		t.Parallel()
		cfg := validProdConfig()
		cfg.CookieSecure = "false"
		require.Panics(t, func() { cfg.validate() })
	})

	t.Run("short csrf", func(t *testing.T) {
		t.Parallel()
		cfg := validProdConfig()
		cfg.CSRFKey = "short"
		require.Panics(t, func() { cfg.validate() })
	})

	t.Run("missing mfa", func(t *testing.T) {
		t.Parallel()
		cfg := validProdConfig()
		cfg.MFASecretKey = ""
		require.Panics(t, func() { cfg.validate() })
	})

	t.Run("short mfa", func(t *testing.T) {
		t.Parallel()
		cfg := validProdConfig()
		cfg.MFASecretKey = "too-short-secret-key-value"
		require.Panics(t, func() { cfg.validate() })
	})

	t.Run("mfa equals csrf", func(t *testing.T) {
		t.Parallel()
		cfg := validProdConfig()
		cfg.MFASecretKey = cfg.CSRFKey
		require.Panics(t, func() { cfg.validate() })
	})
}

// TestConfigValidateOK verifica que configurações válidas de development e production passam em validate.
func TestConfigValidateOK(t *testing.T) {
	t.Parallel()

	assert.NotPanics(t, func() { validDevConfig().validate() })
	assert.NotPanics(t, func() { validProdConfig().validate() })
}

// TestConfigLoadFromEnvDefaults verifica valores padrão aplicados por loadFromEnv quando variáveis estão vazias.
func TestConfigLoadFromEnvDefaults(t *testing.T) {
	t.Setenv("BRIDOPEN_APP_ENV", "")
	t.Setenv("BRIDOPEN_SERVER_PORT", "")
	t.Setenv("BRIDOPEN_BASE_URL", "")
	t.Setenv("BRIDOPEN_DB_CONN_URL", "postgres://test")
	t.Setenv("BRIDOPEN_LEVEL_LOG", "")
	t.Setenv("BRIDOPEN_MAIL_HOST", "mail")
	t.Setenv("BRIDOPEN_MAIL_PORT", "25")
	t.Setenv("BRIDOPEN_MAIL_USERNAME", "user")
	t.Setenv("BRIDOPEN_MAIL_PASSWORD", "pass")
	t.Setenv("BRIDOPEN_MAIL_FROM", "")
	t.Setenv("BRIDOPEN_CSRF_KEY", "csrf")
	t.Setenv("BRIDOPEN_MFA_SECRET_KEY", "")
	t.Setenv("BRIDOPEN_COOKIE_SECURE", "")
	t.Setenv("BRIDOPEN_COOKIE_DOMAIN", "")
	t.Setenv("BRIDOPEN_TRUSTED_ORIGINS", "")
	t.Setenv("BRIDOPEN_ATTACHMENT_STORAGE_DIR", "")
	t.Setenv("BRIDOPEN_CLAMAV_ADDR", "")

	cfg := Config{}.loadFromEnv()
	assert.Equal(t, "development", cfg.AppEnv)
	assert.Equal(t, "5000", cfg.ServerPort)
	assert.Equal(t, "http://localhost:5000", cfg.BaseURL)
	assert.Equal(t, "postgres://test", cfg.DBConnURL)
	assert.Equal(t, "info", cfg.LevelLog)
	assert.Equal(t, "nao-responder@bridopen.local", cfg.MailFrom)
	assert.Equal(t, "false", cfg.CookieSecure)
	assert.Equal(t, "storage/attachments", cfg.AttachmentStorageDir)
}

// TestLoadConfigFromEnv verifica que loadConfig lê corretamente variáveis de ambiente definidas.
func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("BRIDOPEN_APP_ENV", "development")
	t.Setenv("BRIDOPEN_SERVER_PORT", "5000")
	t.Setenv("BRIDOPEN_BASE_URL", "http://localhost:5000")
	t.Setenv("BRIDOPEN_DB_CONN_URL", "postgres://load-config")
	t.Setenv("BRIDOPEN_LEVEL_LOG", "debug")
	t.Setenv("BRIDOPEN_MAIL_HOST", "mail.example")
	t.Setenv("BRIDOPEN_MAIL_PORT", "2525")
	t.Setenv("BRIDOPEN_MAIL_USERNAME", "mailuser")
	t.Setenv("BRIDOPEN_MAIL_PASSWORD", "mailpass")
	t.Setenv("BRIDOPEN_MAIL_FROM", "from@bridopen.com")
	t.Setenv("BRIDOPEN_CSRF_KEY", "csrf-from-env-key")
	t.Setenv("BRIDOPEN_MFA_SECRET_KEY", "")
	t.Setenv("BRIDOPEN_COOKIE_SECURE", "false")
	t.Setenv("BRIDOPEN_COOKIE_DOMAIN", "")
	t.Setenv("BRIDOPEN_TRUSTED_ORIGINS", "")
	t.Setenv("BRIDOPEN_ATTACHMENT_STORAGE_DIR", "tmp/attachments")
	t.Setenv("BRIDOPEN_CLAMAV_ADDR", "")

	cfg := Load()
	assert.Equal(t, "postgres://load-config", cfg.DBConnURL)
	assert.Equal(t, "debug", cfg.LevelLog)
	assert.Equal(t, "tmp/attachments", cfg.AttachmentStorageDir)
}

// TestLoadConfigWithDotEnvFile verifica que loadConfig carrega valores a partir de um arquivo .env.
func TestLoadConfigWithDotEnvFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := "" +
		"BRIDOPEN_APP_ENV=development\n" +
		"BRIDOPEN_SERVER_PORT=5000\n" +
		"BRIDOPEN_BASE_URL=http://localhost:5000\n" +
		"BRIDOPEN_DB_CONN_URL=postgres://from-dotenv\n" +
		"BRIDOPEN_MAIL_HOST=smtp.local\n" +
		"BRIDOPEN_MAIL_PORT=25\n" +
		"BRIDOPEN_MAIL_USERNAME=u\n" +
		"BRIDOPEN_MAIL_PASSWORD=p\n" +
		"BRIDOPEN_CSRF_KEY=dotenv-csrf-key\n"
	require.NoError(t, os.WriteFile(envPath, []byte(content), 0o600))

	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	// godotenv.Load não sobrescreve chaves existentes; remova-as para que os valores do .env se apliquem.
	for _, key := range []string{
		"BRIDOPEN_APP_ENV", "BRIDOPEN_SERVER_PORT", "BRIDOPEN_BASE_URL", "BRIDOPEN_DB_CONN_URL",
		"BRIDOPEN_LEVEL_LOG", "BRIDOPEN_MAIL_HOST", "BRIDOPEN_MAIL_PORT", "BRIDOPEN_MAIL_USERNAME",
		"BRIDOPEN_MAIL_PASSWORD", "BRIDOPEN_MAIL_FROM", "BRIDOPEN_CSRF_KEY", "BRIDOPEN_MFA_SECRET_KEY",
		"BRIDOPEN_COOKIE_SECURE", "BRIDOPEN_COOKIE_DOMAIN", "BRIDOPEN_TRUSTED_ORIGINS",
		"BRIDOPEN_ATTACHMENT_STORAGE_DIR", "BRIDOPEN_CLAMAV_ADDR",
	} {
		orig, had := os.LookupEnv(key)
		require.NoError(t, os.Unsetenv(key))
		k, h, v := key, had, orig
		t.Cleanup(func() {
			if h {
				_ = os.Setenv(k, v)
			} else {
				_ = os.Unsetenv(k)
			}
		})
	}

	cfg := Load()
	assert.Equal(t, "postgres://from-dotenv", cfg.DBConnURL)
	assert.Equal(t, "dotenv-csrf-key", cfg.CSRFKey)
}
