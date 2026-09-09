package commands

import (
	"context"

	"github.com/Hyzokaaa/opencroft/internal/instance/domain/services"
)

type CreateInstanceRequest struct {
	Name     string `json:"name"`
	Image    string `json:"image"`
	Port     int    `json:"port"`
	CPULimit int    `json:"cpuLimit"`
	MemLimit string `json:"memLimit"`
}

type CreateInstanceProps struct {
	Request CreateInstanceRequest
}

type CreateInstanceResponse struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

type CreateInstanceCommand struct {
	createInstance *services.CreateInstance
}

func NewCreateInstanceCommand(createInstance *services.CreateInstance) *CreateInstanceCommand {
	return &CreateInstanceCommand{createInstance: createInstance}
}

func (c *CreateInstanceCommand) Execute(ctx context.Context, props CreateInstanceProps) (CreateInstanceResponse, error) {
	instance, err := c.createInstance.Execute(ctx, services.CreateInstanceProps{
		Name:     props.Request.Name,
		Image:    props.Request.Image,
		Port:     props.Request.Port,
		CPULimit: props.Request.CPULimit,
		MemLimit: props.Request.MemLimit,
	})
	if err != nil {
		return CreateInstanceResponse{}, err
	}

	return CreateInstanceResponse{
		Id:      instance.GetId(),
		Name:    instance.Name,
		Address: instance.Address,
	}, nil
}
