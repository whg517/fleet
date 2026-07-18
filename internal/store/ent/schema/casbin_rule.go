package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CasbinRule holds the schema definition for the CasbinRule entity.
// This table is managed by Casbin PG Adapter; Ent schema defines structure only.
type CasbinRule struct {
	ent.Schema
}

func (CasbinRule) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").Unique().Immutable(),
		field.String("ptype").Optional(),
		field.String("v0").Optional(),
		field.String("v1").Optional(),
		field.String("v2").Optional(),
		field.String("v3").Optional(),
		field.String("v4").Optional(),
		field.String("v5").Optional(),
	}
}

func (CasbinRule) Edges() []ent.Edge {
	return nil
}

func (CasbinRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("ptype"),
		index.Fields("v0", "v1"),
	}
}

// AllowUpdate controls whether UPDATE/DELETE is permitted on this table.
// CasbinRule is managed by Casbin adapter, so we allow updates.
func (CasbinRule) Annotations() []struct{} {
	return nil
}
