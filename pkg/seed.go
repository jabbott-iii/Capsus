/*
Copyright 2026 Joseph Anthony Abbott III

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pkg

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// defaultCategory names the FeatureCategory rows seeded on startup.
type defaultCategory struct {
	Name        string
	Description string
}

// defaultFeature names the Feature rows seeded on startup.
type defaultFeature struct {
	Category    string
	Key         string
	Name        string
	Description string
}

var defaultCategories = []defaultCategory{
	{Name: "System Resources", Description: "Disk, memory, and CPU headroom on the local host."},
	{Name: "Container Runtime", Description: "Docker daemon availability and container health."},
	{Name: "Orchestration", Description: "Kubernetes cluster reachability and status."},
	{Name: "Services", Description: "Uptime of managed services or the host process."},
	{Name: "Configuration", Description: "Common environment misconfigurations."},
	{Name: "Certificates", Description: "Validity of TLS certificate files."},
}

var defaultFeatures = []defaultFeature{
	{Category: "System Resources", Key: keyDiskSpace, Name: "Disk Space", Description: "Verifies free disk space on the configured mount points."},
	{Category: "System Resources", Key: keyMemory, Name: "Memory", Description: "Verifies available memory and swap usage."},
	{Category: "System Resources", Key: keyCPULoad, Name: "CPU Load", Description: "Verifies the system load average relative to available CPUs."},
	{Category: "Container Runtime", Key: keyDocker, Name: "Docker Status", Description: "Verifies the Docker daemon is reachable and no container is unhealthy or restarting."},
	{Category: "Orchestration", Key: keyKubernetes, Name: "Kubernetes Status", Description: "Verifies the Kubernetes cluster is reachable and its nodes are Ready without resource pressure."},
	{Category: "Services", Key: keyServiceUptime, Name: "Service Uptime", Description: "Verifies systemd services (or the host) are up and running."},
	{Category: "Configuration", Key: keyMisconfig, Name: "Misconfiguration", Description: "Scans for common environment misconfigurations."},
	{Category: "System Resources", Key: keyDiskInodes, Name: "Disk Inodes", Description: "Verifies free inodes on the configured mount points."},
	{Category: "Orchestration", Key: keyKubePods, Name: "Kubernetes Pods", Description: "Verifies pods in a namespace are Ready and not crash-looping."},
	{Category: "Services", Key: keySystemdFailed, Name: "Failed Units", Description: "Verifies no systemd unit is in the failed state."},
	{Category: "Services", Key: keyTimeSync, Name: "Time Sync", Description: "Verifies the system clock is synchronized."},
	{Category: "Certificates", Key: keyCertExpiry, Name: "Certificate Expiry", Description: "Verifies certificate files are valid and not close to expiry."},
}

// EnsureDefaultFeatures seeds the built-in feature catalog if it is not already present.
func EnsureDefaultFeatures(db *Database) error {
	return seedCatalog(db.Conn())
}

// seedCatalog inserts the categories and features of the built-in catalog
// that are missing. Existing rows are never updated (intel/maint.md section
// 4), and a catalog that is already complete is only read.
func seedCatalog(conn *gorm.DB) error {
	categoryIDs := make(map[string]uint, len(defaultCategories))
	for _, cat := range defaultCategories {
		record := FeatureCategory{Name: cat.Name, Description: cat.Description}
		if err := firstOrInsert(conn, FeatureCategory{Name: cat.Name}, &record); err != nil {
			return fmt.Errorf("seed category %q: %w", cat.Name, err)
		}
		categoryIDs[cat.Name] = record.ID
	}

	for _, feat := range defaultFeatures {
		categoryID, ok := categoryIDs[feat.Category]
		if !ok {
			return fmt.Errorf("seed feature %q: unknown category %q", feat.Key, feat.Category)
		}

		record := Feature{
			CategoryID:  categoryID,
			Key:         feat.Key,
			Name:        feat.Name,
			Description: feat.Description,
		}
		if err := firstOrInsert(conn, Feature{Key: feat.Key}, &record); err != nil {
			return fmt.Errorf("seed feature %q: %w", feat.Key, err)
		}
	}

	return nil
}

// firstOrInsert loads the row matching where into record, inserting record
// first when no row matches. The insert does nothing on a uniqueness
// conflict, so a row that another connection added in the meantime is read
// back instead of failing the open. When the row exists, nothing is written.
func firstOrInsert[T any](conn *gorm.DB, where T, record *T) error {
	found := conn.Where(where).Limit(1).Find(record)
	if found.Error != nil {
		return found.Error
	}
	if found.RowsAffected > 0 {
		return nil
	}
	if err := conn.Clauses(clause.OnConflict{DoNothing: true}).Create(record).Error; err != nil {
		return err
	}
	return conn.Where(where).First(record).Error
}

// catalogComplete reports whether every feature of the built-in catalog is
// stored. It only reads, so an up-to-date database can be opened read-only.
func catalogComplete(conn *gorm.DB) (bool, error) {
	keys := make([]string, 0, len(defaultFeatures))
	for _, feat := range defaultFeatures {
		keys = append(keys, feat.Key)
	}
	var stored int64
	if err := conn.Model(&Feature{}).Where("key IN ?", keys).Count(&stored).Error; err != nil {
		return false, fmt.Errorf("check feature catalog: %w", err)
	}
	return stored == int64(len(keys)), nil
}
