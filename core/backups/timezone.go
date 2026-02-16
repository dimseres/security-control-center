package backups

import (
	"os"
	"strings"
	"time"
)

const defaultBackupTZ = "Europe/Moscow"

func backupNow() time.Time {
	zone := strings.TrimSpace(os.Getenv("TZ"))
	if zone == "" {
		zone = defaultBackupTZ
	}
	if loc, err := time.LoadLocation(zone); err == nil {
		return time.Now().In(loc)
	}
	if loc, err := time.LoadLocation(defaultBackupTZ); err == nil {
		return time.Now().In(loc)
	}
	return time.Now()
}
