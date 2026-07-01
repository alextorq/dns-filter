package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Logger is the narrow logging port used by the database download handler.
type Logger interface {
	Info(args ...any)
	Error(err error)
}

// Handlers owns every dependency needed by the database HTTP feature. Keeping
// the live connection and the secret-key provider here avoids package globals
// in a security-sensitive endpoint: every exported snapshot is sanitized with
// the settings registry wired by the composition root.
type Handlers struct {
	DB         *gorm.DB
	DBPath     string
	Log        Logger
	SecretKeys func() []string
}

// DownloadDb streams a sanitized snapshot of the SQLite database as an
// attachment. Дамп идёт через `VACUUM INTO` во временный файл, в копии
// удаляются строки таблицы `settings` для всех секретных ключей, а затем
// финальный `VACUUM` перезаписывает файл — свободные страницы (где остаются
// байты удалённого ключа при secure_delete=OFF, дефолте SQLite) уходят,
// `strings filter.sqlite | grep …` секреты не находит.
//
// Временный файл создаётся внутри приватного 0700-каталога (`MkdirTemp`), а не
// в общем `/tmp`: между моментом создания tmp-файла и `VACUUM INTO` другой
// локальный процесс не может подложить symlink на чужую директорию.
// @Summary      Download database file
// @Tags         config
// @Produce      application/octet-stream
// @Success      200 {file} binary "filter.sqlite"
// @Failure      500 {object} map[string]string
// @Router       /api/config/db/download [get]
func (h *Handlers) DownloadDb(c *gin.Context) {
	if err := h.validate(); err != nil {
		if h != nil && h.Log != nil {
			h.Log.Error(fmt.Errorf("download db: invalid handler wiring: %w", err))
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "snapshot failed"})
		return
	}

	// MkdirTemp создаёт каталог с правами 0700 в TMPDIR. Все артефакты дампа
	// (snapshot + потенциальные `-journal`/`-wal`/`-shm` от второго GORM-
	// соединения) живут только внутри него; `os.RemoveAll` в defer уносит
	// каталог целиком — sidecar-файлы не утекают в TMPDIR между запросами.
	tmpDir, err := os.MkdirTemp("", "filter-snapshot-")
	if err != nil {
		h.Log.Error(fmt.Errorf("download db: создание tmp-каталога: %w", err))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "snapshot failed"})
		return
	}
	defer os.RemoveAll(tmpDir)
	tmpPath := filepath.Join(tmpDir, "filter.sqlite")

	if err := snapshotWithoutSecrets(h.DB, tmpPath, h.SecretKeys()); err != nil {
		h.Log.Error(fmt.Errorf("download db: snapshot: %w", err))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "snapshot failed"})
		return
	}

	h.Log.Info("Downloading database file: " + h.DBPath + " (sanitized snapshot)")
	c.FileAttachment(tmpPath, "filter.sqlite")
}

// validate makes incomplete DI fail closed. In particular, a missing
// SecretKeys provider must never degrade to exporting an unsanitized database.
func (h *Handlers) validate() error {
	if h == nil {
		return fmt.Errorf("handlers are nil")
	}
	if h.DB == nil {
		return fmt.Errorf("database is nil")
	}
	if h.Log == nil {
		return fmt.Errorf("logger is nil")
	}
	if h.SecretKeys == nil {
		return fmt.Errorf("secret keys provider is nil")
	}
	return nil
}

// snapshotWithoutSecrets делает атомарную копию live-БД через `VACUUM INTO` и
// удаляет в копии строки `settings.key` ∈ secretKeys, затем компактует копию
// финальным `VACUUM`. `VACUUM INTO` берёт согласованный снимок даже если live-БД
// параллельно пишется — GORM-соединение держится в WAL-режиме, а копия
// сериализуется как обычный SQLite-файл.
//
// Финальный VACUUM на dst критичен: SQLite по умолчанию открыт без
// `secure_delete=ON`, поэтому `DELETE FROM settings WHERE …` только отвязывает
// строку от b-tree, оставляя байты ключа в free-страницах. `VACUUM` переписывает
// файл с нуля — свободных страниц с остаточным содержимым не остаётся.
func snapshotWithoutSecrets(src *gorm.DB, dstPath string, secretKeys []string) error {
	// Имя файла подставляется fmt-сборкой, а не плейсхолдером: SQLite принимает
	// в `VACUUM INTO` только literal-выражение (placeholder отвергается на этапе
	// подготовки запроса). Путь полностью контролируется сервером (MkdirTemp
	// внутри приватного 0700-каталога), инъекция-вектор исключён; escape
	// одинарных кавычек — на случай редких каталогов с апострофом в имени.
	escaped := strings.ReplaceAll(dstPath, "'", "''")
	if err := src.Exec("VACUUM INTO '" + escaped + "'").Error; err != nil {
		return fmt.Errorf("vacuum into snapshot: %w", err)
	}

	if len(secretKeys) == 0 {
		return nil
	}

	// Открываем копию отдельным соединением — Exec на src писать в неё не может
	// (другой файл). Сразу закрываем после операций, чтобы не оставлять
	// журнальные хвосты от ещё открытой транзакции; RemoveAll вокруг каталога
	// гарантирует, что любые потенциальные `-journal` тоже уйдут.
	dst, err := gorm.Open(sqlite.Open(dstPath), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("open snapshot: %w", err)
	}
	defer func() {
		if sqlDB, dberr := dst.DB(); dberr == nil {
			_ = sqlDB.Close()
		}
	}()

	if err := dst.Exec("DELETE FROM settings WHERE key IN ?", secretKeys).Error; err != nil {
		return fmt.Errorf("strip secrets from snapshot: %w", err)
	}
	// Зачистка free-страниц: без этого секрет, удалённый DELETE'ом, остаётся
	// читаемым через `strings snapshot.sqlite` (SQLite default secure_delete=OFF
	// только отмечает страницу как свободную, не перезаписывает байты).
	if err := dst.Exec("VACUUM").Error; err != nil {
		return fmt.Errorf("vacuum snapshot to scrub freed pages: %w", err)
	}
	return nil
}
