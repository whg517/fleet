package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Cluster holds the schema definition for the Cluster entity.
type Cluster struct {
	ent.Schema
}

func (Cluster) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Unique().Immutable(),
		field.String("org_id").Optional(),
		field.String("name").NotEmpty(),
		field.String("api_server").NotEmpty(),
		field.Bytes("kubeconfig_encrypted").Optional(),
		field.JSON("labels", map[string]string{}).Optional(),
		field.Enum("status").Values("active", "inactive").Default("active"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Cluster) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("environments", Environment.Type),
		edge.From("organization", Organization.Type).
			Ref("clusters").
			Field("org_id").
			Unique(),
	}
}

func (Cluster) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("org_id"),
		index.Fields("name"),
		index.Fields("status"),
	}
}
