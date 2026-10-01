package entities

import (
	"github.com/Hyzokaaa/opencroft/internal/instance/domain/enums"
	"github.com/Hyzokaaa/opencroft/internal/shared/id"
)

// Instance is a system container. Pure data — no behaviour lives here.
type Instance struct {
	Id       id.Id
	Name     string
	Image    string
	Address  string
	Port     int
	Domain   string
	CPULimit int
	MemLimit string
	Status   enums.InstanceStatus
	Created  string
	// Project is the one the container belongs to, or empty. It is an
	// annotation on the container, so a migration carries it along.
	Project string

	// Managed is false for containers that exist on the host but were not
	// created by us. They are listed, never touched.
	Managed bool
}

type InstanceProps struct {
	Id       string
	Name     string
	Image    string
	Address  string
	Port     int
	Domain   string
	CPULimit int
	MemLimit string
	Status   enums.InstanceStatus
	Created  string
	Project  string
	Managed  bool
}

func NewInstance(props InstanceProps) *Instance {
	return &Instance{
		Id:       id.New(props.Id),
		Name:     props.Name,
		Image:    props.Image,
		Address:  props.Address,
		Port:     props.Port,
		Domain:   props.Domain,
		CPULimit: props.CPULimit,
		MemLimit: props.MemLimit,
		Status:   props.Status,
		Created:  props.Created,
		Managed:  props.Managed,
		Project:  props.Project,
	}
}

func (i *Instance) GetId() string {
	return i.Id.Get()
}
