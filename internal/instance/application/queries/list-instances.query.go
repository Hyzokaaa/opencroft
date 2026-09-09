package queries

import (
	"context"

	"github.com/Hyzokaaa/opencroft/internal/instance/domain/entities"
	"github.com/Hyzokaaa/opencroft/internal/instance/domain/services"
)

type ListInstancesResponse struct {
	Instances []*entities.Instance
}

type ListInstancesQuery struct {
	listInstances *services.ListInstances
}

func NewListInstancesQuery(listInstances *services.ListInstances) *ListInstancesQuery {
	return &ListInstancesQuery{listInstances: listInstances}
}

func (q *ListInstancesQuery) Execute(ctx context.Context) (ListInstancesResponse, error) {
	found, err := q.listInstances.Execute(ctx)
	if err != nil {
		return ListInstancesResponse{}, err
	}
	return ListInstancesResponse{Instances: found}, nil
}
