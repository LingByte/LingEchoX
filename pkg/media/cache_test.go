package media

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LingByte/LingEchoX/pkg/logger"
)

func init() {
	// Initialize logger for tests
	cfg := &logger.LogConfig{
		Level: "info",
	}
	logger.Init(cfg, "")
}

func TestMediaCache(t *testing.T) {
	// Save original env vars
	originalRoot := os.Getenv("MEDIA_CACHE_ROOT")
	originalDisabled := os.Getenv("MEDIA_CACHE_DISABLED")
	defer func() {
		os.Setenv("MEDIA_CACHE_ROOT", originalRoot)
		os.Setenv("MEDIA_CACHE_DISABLED", originalDisabled)
		_defaultMediaCache = nil
	}()

	t.Run("DefaultInitialization", func(t *testing.T) {
		_defaultMediaCache = nil
		os.Unsetenv("MEDIA_CACHE_ROOT")
		os.Unsetenv("MEDIA_CACHE_DISABLED")

		cache := MediaCache()
		if cache == nil {
			t.Fatal("expected non-nil cache")
		}
		if cache.CacheRoot == "" {
			t.Error("expected non-empty cache root")
		}
	})

	t.Run("CustomRoot", func(t *testing.T) {
		_defaultMediaCache = nil
		tmpDir := t.TempDir()
		os.Setenv("MEDIA_CACHE_ROOT", tmpDir)
		os.Unsetenv("MEDIA_CACHE_DISABLED")

		cache := MediaCache()
		if cache.CacheRoot != tmpDir {
			t.Errorf("expected cache root '%s', got '%s'", tmpDir, cache.CacheRoot)
		}
	})

	t.Run("Disabled", func(t *testing.T) {
		_defaultMediaCache = nil
		os.Setenv("MEDIA_CACHE_DISABLED", "true")

		cache := MediaCache()
		if !cache.Disabled {
			t.Error("expected cache to be disabled")
		}
	})
}

func TestLocalMediaCache_BuildKey(t *testing.T) {
	cache := &LocalMediaCache{}

	t.Run("SingleParam", func(t *testing.T) {
		key := cache.BuildKey("test")
		if key == "" {
			t.Error("expected non-empty key")
		}
		if len(key) != 32 { // MD5 hash length
			t.Errorf("expected key length 32, got %d", len(key))
		}
	})

	t.Run("MultipleParams", func(t *testing.T) {
		key1 := cache.BuildKey("param1", "param2")
		key2 := cache.BuildKey("param1", "param2")
		if key1 != key2 {
			t.Error("expected same key for same params")
		}
	})

	t.Run("DifferentParams", func(t *testing.T) {
		key1 := cache.BuildKey("param1")
		key2 := cache.BuildKey("param2")
		if key1 == key2 {
			t.Error("expected different keys for different params")
		}
	})

	t.Run("EmptyParams", func(t *testing.T) {
		key := cache.BuildKey()
		if key == "" {
			t.Error("expected non-empty key even for empty params")
		}
	})
}

func TestLocalMediaCache_Store(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("StoreSuccess", func(t *testing.T) {
		cache := &LocalMediaCache{
			CacheRoot: tmpDir,
			Disabled:  false,
		}

		data := []byte("test data")
		key := "testkey"
		err := cache.Store(key, data)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify file exists
		filename := filepath.Join(tmpDir, key)
		if _, err := os.Stat(filename); os.IsNotExist(err) {
			t.Error("expected file to exist")
		}
	})

	t.Run("StoreDisabled", func(t *testing.T) {
		cache := &LocalMediaCache{
			CacheRoot: tmpDir,
			Disabled:  true,
		}

		data := []byte("test data")
		key := "testkey2"
		err := cache.Store(key, data)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify file does not exist
		filename := filepath.Join(tmpDir, key)
		if _, err := os.Stat(filename); !os.IsNotExist(err) {
			t.Error("expected file to not exist when disabled")
		}
	})

	t.Run("StoreExistingFile", func(t *testing.T) {
		cache := &LocalMediaCache{
			CacheRoot: tmpDir,
			Disabled:  false,
		}

		key := "existing"
		data1 := []byte("data1")
		data2 := []byte("data2")

		// Store first time
		err := cache.Store(key, data1)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Store again (should overwrite)
		err = cache.Store(key, data2)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify content is updated
		retrieved, err := cache.Get(key)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if string(retrieved) != string(data2) {
			t.Errorf("expected '%s', got '%s'", string(data2), string(retrieved))
		}
	})

	t.Run("StoreDirectory", func(t *testing.T) {
		cache := &LocalMediaCache{
			CacheRoot: tmpDir,
			Disabled:  false,
		}

		// Create a directory with the key name
		key := "dirkey"
		dirPath := filepath.Join(tmpDir, key)
		os.Mkdir(dirPath, 0755)

		data := []byte("test data")
		err := cache.Store(key, data)
		if err != os.ErrExist {
			t.Errorf("expected ErrExist, got %v", err)
		}
	})
}

func TestLocalMediaCache_Get(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("GetSuccess", func(t *testing.T) {
		cache := &LocalMediaCache{
			CacheRoot: tmpDir,
			Disabled:  false,
		}

		data := []byte("test data")
		key := "getkey"
		cache.Store(key, data)

		retrieved, err := cache.Get(key)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if string(retrieved) != string(data) {
			t.Errorf("expected '%s', got '%s'", string(data), string(retrieved))
		}
	})

	t.Run("GetDisabled", func(t *testing.T) {
		cache := &LocalMediaCache{
			CacheRoot: tmpDir,
			Disabled:  true,
		}

		_, err := cache.Get("anykey")
		if err != os.ErrNotExist {
			t.Errorf("expected ErrNotExist, got %v", err)
		}
	})

	t.Run("GetNonExistent", func(t *testing.T) {
		cache := &LocalMediaCache{
			CacheRoot: tmpDir,
			Disabled:  false,
		}

		_, err := cache.Get("nonexistent")
		if err != os.ErrNotExist {
			t.Errorf("expected ErrNotExist, got %v", err)
		}
	})

	t.Run("GetDirectory", func(t *testing.T) {
		cache := &LocalMediaCache{
			CacheRoot: tmpDir,
			Disabled:  false,
		}

		// Create a directory
		key := "getdir"
		dirPath := filepath.Join(tmpDir, key)
		os.Mkdir(dirPath, 0755)

		_, err := cache.Get(key)
		if err != os.ErrNotExist {
			t.Errorf("expected ErrNotExist for directory, got %v", err)
		}
	})
}

func TestLocalMediaCache_Integration(t *testing.T) {
	tmpDir := t.TempDir()
	cache := &LocalMediaCache{
		CacheRoot: tmpDir,
		Disabled:  false,
	}

	// Build key
	key := cache.BuildKey("user123", "audio", "en-US")

	// Store data
	originalData := []byte("audio data here")
	err := cache.Store(key, originalData)
	if err != nil {
		t.Fatalf("failed to store: %v", err)
	}

	// Retrieve data
	retrievedData, err := cache.Get(key)
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}

	// Verify
	if string(retrievedData) != string(originalData) {
		t.Errorf("data mismatch: expected '%s', got '%s'", string(originalData), string(retrievedData))
	}
}
