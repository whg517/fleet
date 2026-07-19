package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SystemSetting holds the schema definition for the SystemSetting entity.
// It stores platform-level configuration such as ArgoCD, Harbor, and Git
// integration settings. Sensitive values (tokens, passwords) are encrypted
// at the application layer using AES-256-GCM.
type SystemSetting struct {
	ent.Schema
}

func (SystemSetting) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Unique().Immutable(),
		field.String("org_id").Optional(),
		field.String("key").NotEmpty(),
		field.String("value").Default(""),
		field.Bool("encrypted").Default(false),
		field.Enum("category").
			Values("argocd", "harbor", "git", "notification", "general").
			Default("general"),
		field.String("description").Optional(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (SystemSetting) Edges() []ent.Edge {
	return nil
}

func (SystemSetting) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("org_id"),
		index.Fields("category"),
		index.Fields("key"),
	}
}
