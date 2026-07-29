package view

import (
	"github.com/google/uuid"
	"fmt"
	reapi "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	bfpb "github.com/buildfarm/buildfarm/build/buildfarm/v1test"
	"google.golang.org/genproto/googleapis/longrunning"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/buildfarm/bf-client/client"
)

type run struct {
	metadata reapi.RequestMetadata
	request reapi.ExecuteRequest
	count int
	outstanding longrunning.Operation
	p *widgets.Paragraph
	c client.Component
	v View
}

func defaultMetadata(a *client.App, actionId string, configurationId string) reapi.RequestMetadata {
	invocationId := uuid.New()

	return reapi.RequestMetadata {
		ToolDetails: &reapi.ToolDetails {
			ToolName: "bf-client",
			ToolVersion: a.Version,
		},
		ActionId: actionId,
		ToolInvocationId: invocationId.String(),
		CorrelatedInvocationsId: a.Id.String(),
		ActionMnemonic: "Action",
		ConfigurationId: configurationId,
	}
}

func defaultRequest(a *client.App, actionDigest bfpb.Digest) reapi.ExecuteRequest {
	r := reapi.ExecuteRequest {
		InstanceName: a.Instance,
		SkipCacheLookup: true,
		ActionDigest: &reapi.Digest { },
		ExecutionPolicy: &reapi.ExecutionPolicy {
			Priority: 0,
		},
		ResultsCachePolicy: &reapi.ResultsCachePolicy {
			Priority: 0,
		},
		DigestFunction: actionDigest.DigestFunction,
		InlineStdout: false,
		InlineStderr: false,
		InlineOutputFiles: []string { },
	}
	*r.ActionDigest = client.FromDigest(actionDigest)
	return r
}

func NewRun(actionDigest bfpb.Digest, c client.Component, v View) *run {
	// form
	p := widgets.NewParagraph()
	hashFn := client.HasherFromDigestFunction(actionDigest.DigestFunction)
	configuration, _ := client.DigestFromMessage(&c.App().Capabilities, hashFn)
	metadata := defaultMetadata(c.App(), actionDigest.Hash, configuration.Hash)
	request := defaultRequest(c.App(), actionDigest)

	p.Title = fmt.Sprintf("%s - %s", client.DigestString(actionDigest), metadata.ToolInvocationId)
	return &run {
		request: request,
		p: p,
		c: c,
		v: v,
	}
}

func (r *run) Update() {
}

func (r run) Render() []ui.Drawable {
	return []ui.Drawable { r.p }
}

func (r *run) Handle(e ui.Event) View {
	return r.v
}
