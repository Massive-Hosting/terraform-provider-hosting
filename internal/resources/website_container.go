package resources

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

/*
The container runtime of a website.

A container is a website whose runtime is "container": the image, its limits
and its ports are runtime_config, the same field every other runtime uses for
its own settings. Writing that by hand as jsonencode() would be miserable, so
the website resource carries a typed `container` block and this file converts
between it and the JSON the API stores.

Anything not mentioned here — domains, env vars, status — is a website concern
and already has a resource.
*/

// containerSpecModel is the `container` block on hosting_website.
type containerSpecModel struct {
	Image             types.String          `tfsdk:"image"`
	Command           types.String          `tfsdk:"command"`
	ImagePullSecretID types.String          `tfsdk:"image_pull_secret_id"`
	RestartPolicy     types.String          `tfsdk:"restart_policy"`
	MaxMemoryMB       types.Int64           `tfsdk:"max_memory_mb"`
	MaxCPUCores       types.Float64         `tfsdk:"max_cpu_cores"`
	ProxyPath         types.String          `tfsdk:"proxy_path"`
	ProxyPort         types.Int64           `tfsdk:"proxy_port"`
	Ports             []containerPortModel  `tfsdk:"ports"`
	Volumes           []containerMountModel `tfsdk:"volumes"`
	Health            *containerHealthModel `tfsdk:"health_check"`
}

type containerHealthModel struct {
	Type           types.String `tfsdk:"type"`
	Path           types.String `tfsdk:"path"`
	Port           types.Int64  `tfsdk:"port"`
	TimeoutSeconds types.Int64  `tfsdk:"timeout_seconds"`
	Retries        types.Int64  `tfsdk:"retries"`
}

type containerPortModel struct {
	ContainerPort types.Int64  `tfsdk:"container_port"`
	HostPort      types.Int64  `tfsdk:"host_port"`
	Protocol      types.String `tfsdk:"protocol"`
}

type containerMountModel struct {
	HostPath      types.String `tfsdk:"host_path"`
	ContainerPath types.String `tfsdk:"container_path"`
	ReadOnly      types.Bool   `tfsdk:"read_only"`
}

// containerSpecJSON mirrors model.ContainerSpec in the platform.
type containerSpecJSON struct {
	Image             string               `json:"image"`
	Command           *string              `json:"command,omitempty"`
	ImagePullSecretID *string              `json:"image_pull_secret_id,omitempty"`
	PortMappings      []containerPortJSON  `json:"port_mappings"`
	VolumeMounts      []containerVolJSON   `json:"volume_mounts"`
	RestartPolicy     string               `json:"restart_policy"`
	MaxMemoryMB       int64                `json:"max_memory_mb"`
	MaxCPUCores       float64              `json:"max_cpu_cores"`
	ProxyPort         *int64               `json:"proxy_port,omitempty"`
	ProxyPath         *string              `json:"proxy_path,omitempty"`
	HealthCheck       *containerHealthJSON `json:"health_check,omitempty"`
}

type containerHealthJSON struct {
	Type           string `json:"type"`
	Path           string `json:"path,omitempty"`
	Port           int64  `json:"port,omitempty"`
	TimeoutSeconds int64  `json:"timeout_seconds,omitempty"`
	Retries        int64  `json:"retries,omitempty"`
}

type containerPortJSON struct {
	ContainerPort int64  `json:"container_port"`
	HostPort      int64  `json:"host_port,omitempty"`
	Protocol      string `json:"protocol"`
}

type containerVolJSON struct {
	HostPath      string `json:"host_path"`
	ContainerPath string `json:"container_path"`
	ReadOnly      bool   `json:"read_only"`
}

// containerSchema is the `container` attribute on hosting_website.
func containerSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional: true,
		Description: "Container settings. Required when runtime is \"container\", " +
			"ignored otherwise. These are stored as the website's runtime_config.",
		Attributes: map[string]schema.Attribute{
			"image": schema.StringAttribute{
				Required:    true,
				Description: "Image reference including the tag (e.g. ghcr.io/you/api:1.8.2).",
			},
			"command": schema.StringAttribute{
				Optional:    true,
				Description: "Overrides the image ENTRYPOINT. Omit to use what the image defines.",
			},
			"image_pull_secret_id": schema.StringAttribute{
				Optional:    true,
				Description: "Registry credential to pull with, for a private image.",
			},
			"restart_policy": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "What happens when the process exits: always, on-failure, unless-stopped or no.",
			},
			"max_memory_mb": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Hard memory ceiling in MB. A container that exceeds it is killed with exit code 137.",
			},
			"max_cpu_cores": schema.Float64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "CPU ceiling in cores.",
			},
			"proxy_path": schema.StringAttribute{
				Optional:    true,
				Description: "Serve the container under this path on the node, for a container with no domain of its own.",
			},
			"proxy_port": schema.Int64Attribute{
				Computed:    true,
				Description: "The host port the platform derives for this container. Not settable.",
			},
			"ports": schema.ListNestedAttribute{
				Optional:    true,
				Description: "Ports the container listens on.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"container_port": schema.Int64Attribute{
							Required:    true,
							Description: "Port inside the container.",
						},
						"host_port": schema.Int64Attribute{
							Optional: true,
							Computed: true,
							Description: "Port published on the workload's internal address. " +
								"Defaults to container_port. This is the port a connected " +
								"website reaches the service on, so it is what the generated " +
								"_PORT variable and the health check use.",
						},
						"protocol": schema.StringAttribute{
							Optional:    true,
							Computed:    true,
							Description: "tcp or udp.",
						},
					},
				},
			},
			"health_check": schema.SingleNestedAttribute{
				Optional: true,
				Description: "How the platform decides a rollout worked. Probed from outside the " +
					"container, so the image needs no health endpoint or shell. Without one, a " +
					"container that starts and immediately crashes is reported as running.",
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Required:    true,
						Description: "tcp (the port accepts a connection) or http (a request to path returns a non-5xx).",
					},
					"path": schema.StringAttribute{
						Optional:    true,
						Computed:    true,
						Description: "Path to request for an http check. Defaults to /.",
					},
					"port": schema.Int64Attribute{
						Optional:    true,
						Computed:    true,
						Description: "Defaults to the first declared port.",
					},
					"timeout_seconds": schema.Int64Attribute{
						Optional:    true,
						Computed:    true,
						Description: "Bounds one attempt. Default 3.",
					},
					"retries": schema.Int64Attribute{
						Optional:    true,
						Computed:    true,
						Description: "Attempts, one second apart. Together with the timeout this is how long a slow boot may take. Default 30.",
					},
				},
			},
			"volumes": schema.ListNestedAttribute{
				Optional:    true,
				Description: "Persistent volumes mounted into the container.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"host_path": schema.StringAttribute{
							Required:    true,
							Description: "Path within the tenant's storage.",
						},
						"container_path": schema.StringAttribute{
							Required:    true,
							Description: "Where it appears inside the container.",
						},
						"read_only": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Mount read-only.",
						},
					},
				},
			},
		},
	}
}

// containerRuntimeConfig renders the block as the JSON the API stores.
//
// The derived proxy_port is carried over from prior state rather than sent by
// the practitioner: the platform computes it, and dropping it on an update
// would take the container's published port away.
func containerRuntimeConfig(spec *containerSpecModel, priorProxyPort types.Int64) (json.RawMessage, error) {
	if spec == nil {
		return nil, fmt.Errorf("runtime is \"container\" but no container block is set")
	}

	out := containerSpecJSON{
		Image:         spec.Image.ValueString(),
		RestartPolicy: valueOr(spec.RestartPolicy.ValueString(), "always"),
		MaxMemoryMB:   valueOr(spec.MaxMemoryMB.ValueInt64(), 512),
		MaxCPUCores:   valueOr(spec.MaxCPUCores.ValueFloat64(), 1.0),
		PortMappings:  []containerPortJSON{},
		VolumeMounts:  []containerVolJSON{},
	}
	if !spec.Command.IsNull() && spec.Command.ValueString() != "" {
		cmd := spec.Command.ValueString()
		out.Command = &cmd
	}
	if !spec.ImagePullSecretID.IsNull() && spec.ImagePullSecretID.ValueString() != "" {
		id := spec.ImagePullSecretID.ValueString()
		out.ImagePullSecretID = &id
	}
	if !spec.ProxyPath.IsNull() && spec.ProxyPath.ValueString() != "" {
		path := spec.ProxyPath.ValueString()
		out.ProxyPath = &path
	}
	switch {
	case !spec.ProxyPort.IsNull() && !spec.ProxyPort.IsUnknown() && spec.ProxyPort.ValueInt64() != 0:
		port := spec.ProxyPort.ValueInt64()
		out.ProxyPort = &port
	case !priorProxyPort.IsNull() && !priorProxyPort.IsUnknown() && priorProxyPort.ValueInt64() != 0:
		port := priorProxyPort.ValueInt64()
		out.ProxyPort = &port
	}
	for _, p := range spec.Ports {
		out.PortMappings = append(out.PortMappings, containerPortJSON{
			ContainerPort: p.ContainerPort.ValueInt64(),
			HostPort:      p.HostPort.ValueInt64(),
			Protocol:      valueOr(p.Protocol.ValueString(), "tcp"),
		})
	}
	for _, v := range spec.Volumes {
		out.VolumeMounts = append(out.VolumeMounts, containerVolJSON{
			HostPath:      v.HostPath.ValueString(),
			ContainerPath: v.ContainerPath.ValueString(),
			ReadOnly:      v.ReadOnly.ValueBool(),
		})
	}
	if h := spec.Health; h != nil {
		out.HealthCheck = &containerHealthJSON{
			Type:           h.Type.ValueString(),
			Path:           h.Path.ValueString(),
			Port:           h.Port.ValueInt64(),
			TimeoutSeconds: h.TimeoutSeconds.ValueInt64(),
			Retries:        h.Retries.ValueInt64(),
		}
	}

	raw, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("encode container block: %w", err)
	}
	return raw, nil
}

// containerSpecFromJSON reads the stored runtime_config back into the block,
// so a container imported or changed outside Terraform reads correctly.
func containerSpecFromJSON(raw json.RawMessage) *containerSpecModel {
	if len(raw) == 0 {
		return nil
	}
	var in containerSpecJSON
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil
	}

	spec := &containerSpecModel{
		Image:             types.StringValue(in.Image),
		Command:           stringOrNull(in.Command),
		ImagePullSecretID: stringOrNull(in.ImagePullSecretID),
		RestartPolicy:     types.StringValue(in.RestartPolicy),
		MaxMemoryMB:       types.Int64Value(in.MaxMemoryMB),
		MaxCPUCores:       types.Float64Value(in.MaxCPUCores),
		ProxyPath:         stringOrNull(in.ProxyPath),
		ProxyPort:         types.Int64Null(),
		Ports:             []containerPortModel{},
		Volumes:           []containerMountModel{},
	}
	if in.ProxyPort != nil {
		spec.ProxyPort = types.Int64Value(*in.ProxyPort)
	}
	for _, p := range in.PortMappings {
		spec.Ports = append(spec.Ports, containerPortModel{
			ContainerPort: types.Int64Value(p.ContainerPort),
			// The API omits host_port when it equals the container port, but
			// the attribute is computed, so state has to carry the number the
			// platform actually publishes rather than a null.
			HostPort: types.Int64Value(publishedOr(p.HostPort, p.ContainerPort)),
			Protocol: types.StringValue(p.Protocol),
		})
	}
	for _, v := range in.VolumeMounts {
		spec.Volumes = append(spec.Volumes, containerMountModel{
			HostPath:      types.StringValue(v.HostPath),
			ContainerPath: types.StringValue(v.ContainerPath),
			ReadOnly:      types.BoolValue(v.ReadOnly),
		})
	}
	if h := in.HealthCheck; h != nil {
		spec.Health = &containerHealthModel{
			Type:           types.StringValue(h.Type),
			Path:           stringOrNull(&h.Path),
			Port:           types.Int64Value(h.Port),
			TimeoutSeconds: types.Int64Value(h.TimeoutSeconds),
			Retries:        types.Int64Value(h.Retries),
		}
	}
	return spec
}

func stringOrNull(s *string) types.String {
	if s == nil || *s == "" {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

// valueOr substitutes a default for the zero value, matching what the platform
// fills in, so the plan does not show a diff against it.
func valueOr[T comparable](v, fallback T) T {
	var zero T
	if v == zero {
		return fallback
	}
	return v
}

var _ = context.Background

// publishedOr returns the host port, falling back to the container port — the
// same rule the platform applies when a mapping declares no host port.
func publishedOr(hostPort, containerPort int64) int64 {
	if hostPort != 0 {
		return hostPort
	}
	return containerPort
}
