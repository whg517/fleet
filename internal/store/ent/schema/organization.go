package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Organization holds the schema definition for the Organization entity.
type Organization struct {
	ent.Schema
}

func (Organization) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Unique().Immutable(),
		field.String("name").NotEmpty(),
		field.String("slug").Unique(),
		field.String("description").Optional(),
		field.Enum("status").Values("active", "disabled").Default("active"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Organization) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("users", User.Type),
		edge.To("clusters", Cluster.Type),
		edge.To("environments", Environment.Type),
		edge.To("registries", Registry.Type),
		edge.To("services", Service.Type),
	}
}

func (Organization) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("slug"),
		index.Fields("status"),
	}
}
