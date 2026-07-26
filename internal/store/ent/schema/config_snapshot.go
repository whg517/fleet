package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ConfigSnapshot holds the schema definition for the ConfigSnapshot entity.
// A ConfigSnapshot records a Helm values change at the environment level,
// capturing who/when/what (old_values + new_values) for audit history.
type ConfigSnapshot struct {
	ent.Schema
}

func (ConfigSnapshot) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Unique().Immutable(),
		field.String("org_id").Optional(),
		field.String("service_id").NotEmpty(),
		field.String("environment_id").NotEmpty(),
		field.JSON("values", map[string]any{}).Optional(),
		field.JSON("previous_values", map[string]any{}).Optional(),
		field.String("changed_by").Optional(),
		field.String("change_reason").Optional(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (ConfigSnapshot) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("service", Service.Type).
			Ref("config_snapshots").
			Field("service_id").
			Unique().
			Required(),
		edge.From("environment", Environment.Type).
			Ref("config_snapshots").
			Field("environment_id").
			Unique().
			Required(),
		edge.From("organization", Organization.Type).
			Ref("config_snapshots").
			Field("org_id").
			Unique(),
	}
}

func (ConfigSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("org_id"),
		index.Fields("service_id"),
		index.Fields("environment_id"),
		index.Fields("service_id", "environment_id"),
	}
}
