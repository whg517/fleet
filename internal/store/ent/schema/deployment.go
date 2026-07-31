package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Deployment holds the schema definition for the Deployment entity.
// A Deployment represents a single deploy action: creating an Argo CD
// Application for a specific Service in a specific Environment using a
// TemplateVersion (Helm Chart) at a given image tag/version.
type Deployment struct {
	ent.Schema
}

func (Deployment) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Unique().Immutable(),
		field.String("org_id").Optional(),
		field.String("service_id").NotEmpty(),
		field.String("environment_id").NotEmpty(),
		field.String("cluster_id").NotEmpty(),
		field.String("template_version_id").NotEmpty(),
		field.String("version").NotEmpty(), // image tag / version being deployed
		field.JSON("values_override", map[string]any{}).Optional(),
		field.Enum("status").Values(
			"pending",
			"validating",
			"deploying",
			"healthy",
			"degraded",
			"failed",
			"cancelled",
		).Default("pending"),
		field.String("argocd_app_name").Optional(),
		field.String("sync_status").Optional(),
		field.String("health_status").Optional(),
		field.String("created_by").Optional(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
		field.Time("completed_at").Optional().Nillable(),
	}
}

func (Deployment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("service", Service.Type).
			Ref("deployments").
			Field("service_id").
			Unique().
			Required(),
		edge.From("environment", Environment.Type).
			Ref("deployments").
			Field("environment_id").
			Unique().
			Required(),
		edge.From("cluster", Cluster.Type).
			Ref("deployments").
			Field("cluster_id").
			Unique().
			Required(),
		edge.From("organization", Organization.Type).
			Ref("deployments").
			Field("org_id").
			Unique(),
		edge.To("approvals", Approval.Type),
	}
}

func (Deployment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("org_id"),
		index.Fields("service_id"),
		index.Fields("environment_id"),
		index.Fields("status"),
		index.Fields("service_id", "environment_id"),
	}
}
