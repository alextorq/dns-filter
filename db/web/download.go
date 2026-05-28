package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/alextorq/dns-filter/config"
	app_db "github.com/alextorq/dns-filter/db"
	"github.com/alextorq/dns-filter/logger"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// secretKeysProvider возвращает ключи секретных настроек, которые нужно
// вырезать из выгружаемой копии БД. Выставляется в main.go через
// SetSecretKeysProvider; nil-значение трактуется как "секретов нет".
//
// Тип-хук, а не зависимость в Handlers, чтобы оставить пакет в текущем
// package-level-Register-стиле (см. ARCHITECTURE.md «Самонастройка маршрутов»).
var secretKeysProvider func() []string

// SetSecretKeysProvider — точка инжекции из composition root.
func SetSecretKeysProvider(f func() []string) { secretKeysProvider = f }

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
func DownloadDb(c *gin.Context) {
	l := logger.GetLogger()
	conf := config.GetConfig()

	// MkdirTemp создаёт каталог с правами 0700 в TMPDIR. Все артефакты дампа
	// (snapshot + потенциальные `-journal`/`-wal`/`-shm` от второго GORM-
	// соединения) живут только внутри него; `os.RemoveAll` в defer уносит
	// каталог целиком — sidecar-файлы не утекают в TMPDIR между запросами.
	tmpDir, err := os.MkdirTemp("", "filter-snapshot-")
	if err != nil {
		l.Error(fmt.Errorf("download db: создание tmp-каталога: %w", err))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "snapshot failed"})
		return
	}
	defer os.RemoveAll(tmpDir)
	tmpPath := filepath.Join(tmpDir, "filter.sqlite")

	if err := snapshotWithoutSecrets(app_db.GetConnection(), tmpPath, secretKeysSafe()); err != nil {
		l.Error(fmt.Errorf("download db: snapshot: %w", err))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "snapshot failed"})
		return
	}

	l.Info("Downloading database file: " + conf.DbPath + " (sanitized snapshot)")
	c.FileAttachment(tmpPath, "filter.sqlite")
}

// secretKeysSafe защищается от nil-провайдера: если main.go не подключил его,
// возвращаем пустой срез — никакие строки не удаляются, поведение совместимо
// с прежним эндпоинтом.
func secretKeysSafe() []string {
	if secretKeysProvider == nil {
		return nil
	}
	return secretKeysProvider()
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
