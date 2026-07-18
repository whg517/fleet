package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AuditLog holds the schema definition for the AuditLog entity.
// INSERT-only table with hash chain for tamper resistance.
// Application accounts should have no UPDATE/DELETE permission at the DB level.
type AuditLog struct {
	ent.Schema
}

func (AuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Unique().Immutable(),
		field.String("user_id").Optional(),
		field.String("action").NotEmpty(),
		field.String("resource_type").NotEmpty(),
		field.String("resource_id").Optional(),
		field.JSON("detail", map[string]any{}).Optional(),
		field.String("ip").Optional(),
		field.String("prev_hash").Optional(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (AuditLog) Edges() []ent.Edge {
	return nil
}

func (AuditLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("resource_type", "resource_id"),
		index.Fields("action"),
		index.Fields("created_at"),
	}
}
