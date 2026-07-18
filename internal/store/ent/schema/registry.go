package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Registry holds the schema definition for the Registry entity.
type Registry struct {
	ent.Schema
}

func (Registry) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Unique().Immutable(),
		field.String("org_id").Optional(),
		field.String("name").NotEmpty(),
		field.Enum("type").Values("harbor").Default("harbor"),
		field.String("url").NotEmpty(),
		field.String("credentials_ref").Optional(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Registry) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("organization", Organization.Type).
			Ref("registries").
			Field("org_id").
			Unique(),
	}
}

func (Registry) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("org_id"),
		index.Fields("name"),
		index.Fields("type"),
	}
}
