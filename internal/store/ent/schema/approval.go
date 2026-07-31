package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Approval holds the schema definition for the Approval entity.
// An Approval is created when a deployment targets an environment
// that requires approval (e.g. pre/prod). It tracks the request,
// the approver decision, and timeout for auto-rejection.
type Approval struct {
	ent.Schema
}

func (Approval) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Unique().Immutable(),
		field.String("org_id").Optional(),
		field.String("deployment_id").NotEmpty(),
		field.String("service_id").NotEmpty(),
		field.String("environment_id").NotEmpty(),
		field.String("requester_id").NotEmpty(),
		field.String("approver_id").Optional(),
		field.Enum("status").Values(
			"pending",
			"approved",
			"rejected",
			"timeout",
			"cancelled",
		).Default("pending"),
		field.Time("timeout_at"),
		field.Time("decided_at").Optional().Nillable(),
		field.String("comment").Optional(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Approval) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("deployment", Deployment.Type).
			Ref("approvals").
			Field("deployment_id").
			Unique().
			Required(),
		edge.From("service", Service.Type).
			Ref("approvals").
			Field("service_id").
			Unique().
			Required(),
		edge.From("environment", Environment.Type).
			Ref("approvals").
			Field("environment_id").
			Unique().
			Required(),
		edge.From("organization", Organization.Type).
			Ref("approvals").
			Field("org_id").
			Unique(),
	}
}

func (Approval) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("org_id"),
		index.Fields("deployment_id"),
		index.Fields("service_id"),
		index.Fields("environment_id"),
		index.Fields("status"),
		index.Fields("service_id", "environment_id"),
	}
}
