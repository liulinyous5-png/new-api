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
package authz

const ResourceEndpointAudit = "endpoint_audit"

var (
	EndpointAuditRead    = Permission{Resource: ResourceEndpointAudit, Action: ActionRead}
	EndpointAuditOperate = Permission{Resource: ResourceEndpointAudit, Action: ActionOperate}
)

func init() {
	RegisterResource(ResourceDefinition{
		Resource: ResourceEndpointAudit,
		LabelKey: "Endpoint audit",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "View endpoint audits",
				DescriptionKey: "View endpoint probes, suites, and audit results. API keys are not included.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionOperate,
				LabelKey:       "Run endpoint audits",
				DescriptionKey: "Send a saved suite to an HTTP or HTTPS endpoint with a key that is not stored.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
		},
	})
}
