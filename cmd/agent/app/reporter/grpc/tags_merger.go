package grpc

import (
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	"github.com/jaegertracing/jaeger/model"
)

type TagsMerger interface {
	// Merge merges jaeger tags for the agent to every span it sends to the collector based on a policy
	Merge(spanTags, agentTags []model.KeyValue) []model.KeyValue
}

type tagsMerger struct {
	duplicateTagsPolicy string
}

// DeDupeAgentTagsMerger creates a TagsMerger with Agent dedupe policy
func DeDupeAgentTagsMerger() TagsMerger {
	return NewTagsMerger(reporter.Agent)
}

// DeDupeClientTagsMerger creates a TagsMerger with Client dedupe policy
func DeDupeClientTagsMerger() TagsMerger {
	return NewTagsMerger(reporter.Client)
}

// SimpleTagsMerger creates a TagsMerger with Duplicate dedupe policy
func SimpleTagsMerger() TagsMerger {
	return NewTagsMerger(reporter.Duplicate)
}

// NewTagsMerger returns a TagsMerger based on the policy
func NewTagsMerger(duplicateTagsPolicy string) TagsMerger {
	return &tagsMerger{
		duplicateTagsPolicy: duplicateTagsPolicy,
	}
}

// Merge merges jaeger tags for the agent to every span it sends to the collector based on a policy
func (tm *tagsMerger) Merge(spanTags, agentTags []model.KeyValue) []model.KeyValue {

	if tm.duplicateTagsPolicy != reporter.Duplicate {
		for _, agentTag := range agentTags {
			index, alreadyPresent := checkIfPresentAlready(spanTags, agentTag)
			if alreadyPresent {
				// If Policy is to keep agent tags, purge duplicate client tag and add AgentTag. Else Do Nothing
				if tm.duplicateTagsPolicy == reporter.Agent {
					// remove index from Tags and add agentTag
					spanTags = append(spanTags[:index], spanTags[index+1:]...)
					spanTags = append(spanTags, agentTag)
				}
			} else {
				spanTags = append(spanTags, agentTag)
			}
		}
	} else {
		spanTags = append(spanTags, agentTags...)
	}

	return spanTags
}

func checkIfPresentAlready(tags []model.KeyValue, agentTag model.KeyValue) (int, bool) {
	for i := range tags {
		if tags[i].Key == agentTag.Key {
			return i, true
		}
	}
	return 0, false
}
