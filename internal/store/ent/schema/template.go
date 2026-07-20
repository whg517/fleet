package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Template holds the schema definition for the Template entity.
// A Template represents a Helm Chart, Ansible Role, or Argo WorkflowTemplate
// used for deployment, build, and configuration management.
type Template struct {
	ent.Schema
}

func (Template) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Unique().Immutable(),
		field.String("org_id").Optional(),
		field.String("name").NotEmpty(),
		field.Enum("type").Values("build", "deploy_k8s", "deploy_vm").Default("deploy_k8s"),
		field.Enum("source").Values("platform", "external_oci").Default("platform"),
		field.String("registry_id").Optional(),
		field.String("repo").Optional(),
		field.String("description").Optional(),
		field.Enum("status").Values("active", "archived").Default("active"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Template) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("versions", TemplateVersion.Type),
		edge.From("organization", Organization.Type).
			Ref("templates").
			Field("org_id").
			Unique(),
	}
}

func (Template) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("org_id", "name").Unique(),
		index.Fields("type"),
		index.Fields("source"),
		index.Fields("status"),
	}
}
