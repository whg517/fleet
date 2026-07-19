package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Service holds the schema definition for the Service entity (microservice catalog).
type Service struct {
	ent.Schema
}

func (Service) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Unique().Immutable(),
		field.String("org_id").Optional(),
		field.String("name").NotEmpty(),
		field.String("team").Optional(),
		field.String("description").Optional(),
		field.JSON("labels", map[string]string{}).Optional(),
		field.Enum("status").Values("active", "archived").Default("active"),
		field.String("harbor_project").Optional(),
		field.String("git_repo").Optional(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Service) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("organization", Organization.Type).
			Ref("services").
			Field("org_id").
			Unique(),
	}
}

func (Service) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("org_id", "name").Unique(),
		index.Fields("org_id"),
		index.Fields("name"),
		index.Fields("team"),
		index.Fields("status"),
	}
}
