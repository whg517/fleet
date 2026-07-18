package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Environment holds the schema definition for the Environment entity.
type Environment struct {
	ent.Schema
}

func (Environment) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Unique().Immutable(),
		field.String("org_id").Optional(),
		field.Enum("name").Values("dev", "test", "pre", "prod"),
		field.String("cluster_id").Optional(),
		field.String("namespace_pattern").Optional(),
		field.Bool("approval_required").Default(false),
		field.String("approver_role").Optional(),
		field.JSON("config_overrides", map[string]any{}).Optional(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Environment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("cluster", Cluster.Type).
			Ref("environments").
			Field("cluster_id").
			Unique(),
		edge.From("organization", Organization.Type).
			Ref("environments").
			Field("org_id").
			Unique(),
	}
}

func (Environment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("org_id"),
		index.Fields("cluster_id"),
		index.Fields("name"),
	}
}
