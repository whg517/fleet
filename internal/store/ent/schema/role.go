package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// Role holds the schema definition for the Role entity.
type Role struct {
	ent.Schema
}

func (Role) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Unique().Immutable(),
		field.Enum("name").
			Values("admin", "operator", "developer", "viewer", "auditor"),
		field.String("description").Optional(),
	}
}

func (Role) Edges() []ent.Edge {
	return nil
}
