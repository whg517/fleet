package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TemplateVersion holds the schema definition for the TemplateVersion entity.
// Each TemplateVersion is an immutable, published version of a Template
// (e.g., a specific Helm Chart version identified by semver + digest).
type TemplateVersion struct {
	ent.Schema
}

func (TemplateVersion) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Unique().Immutable(),
		field.String("template_id"),
		field.String("version").NotEmpty(),
		field.String("digest").Optional(),
		field.JSON("values_schema", map[string]any{}).Optional(),
		field.Text("changelog").Optional(),
		field.Enum("status").Values("active", "archived").Default("active"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (TemplateVersion) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("template", Template.Type).
			Ref("versions").
			Field("template_id").
			Unique().
			Required(),
	}
}

func (TemplateVersion) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("template_id"),
		index.Fields("template_id", "version").Unique(),
		index.Fields("status"),
	}
}
