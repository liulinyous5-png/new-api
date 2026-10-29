/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package model

type EndpointAuditRun struct {
	Id         int    `json:"id" gorm:"primaryKey"`
	SuiteId    int    `json:"suite_id" gorm:"index"`
	SuiteName  string `json:"suite_name" gorm:"size:128"`
	BaseHost   string `json:"base_host" gorm:"size:255"`
	Verdict    string `json:"verdict" gorm:"size:32;index"`
	Error      string `json:"error" gorm:"type:text"`
	Items      string `json:"items" gorm:"type:text"`
	CreatedAt  int64  `json:"created_at" gorm:"index"`
	FinishedAt int64  `json:"finished_at"`
}

func (EndpointAuditRun) TableName() string { return "endpoint_audit_runs" }

func MigrateEndpointAudit() error {
	if err := DB.AutoMigrate(&EndpointAuditRun{}); err != nil {
		return err
	}
	return DB.Where("id > ?", 0).Delete(&EndpointAuditRun{}).Error
}
