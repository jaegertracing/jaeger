package grpc

import (
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	"github.com/jaegertracing/jaeger/model"
)

type TagsMerger interface {
	// Merge merges jaeger tags for the agent to every span it sends to the collector based on a policy
	Merge(spans []*model.Span, process *model.Process) ([]*model.Span, *model.Process)
}

type tagsMerger struct {
	agentTags           []model.KeyValue
	duplicateTagsPolicy string
}

func NewTagsMerger(agentTags []model.KeyValue, duplicateTagsPolicy string) TagsMerger {
	return &tagsMerger{
		agentTags:           agentTags,
		duplicateTagsPolicy: duplicateTagsPolicy,
	}
}

// Merge merges jaeger tags for the agent to every span it sends to the collector based on a policy
func (tm *tagsMerger) Merge(spans []*model.Span, process *model.Process) ([]*model.Span, *model.Process) {
	if len(tm.agentTags) == 0 {
		return spans, process
	}
	if process != nil {
		process.Tags = append(process.Tags, tm.agentTags...)
	}
	for _, span := range spans {
		if span.Process != nil {
			if tm.duplicateTagsPolicy != reporter.Duplicate {
				for _, agentTag := range tm.agentTags {
					index, alreadyPresent := checkIfPresentAlready(span.Process.Tags, agentTag)
					if alreadyPresent {
						// If Policy is to keep agent tags, purge duplicate client tag and add AgentTag. Else Do Nothing
						if tm.duplicateTagsPolicy == reporter.Agent {
							// remove index from Tags and add agentTag
							span.Process.Tags = append(span.Process.Tags[:index], span.Process.Tags[index+1:]...)
							span.Process.Tags = append(span.Process.Tags, agentTag)
						}
					} else {
						span.Process.Tags = append(span.Process.Tags, agentTag)
					}
				}
			} else {
				span.Process.Tags = append(span.Process.Tags, tm.agentTags...)
			}
		}
	}
	return spans, process
}
