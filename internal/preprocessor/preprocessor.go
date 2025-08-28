package preprocessor

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"llm-message-queue/pkg/config"
	"llm-message-queue/pkg/models"
)

// Preprocessor analyzes message content and determines priority
type Preprocessor struct {
	keywordPatterns map[models.Priority][]*regexp.Regexp
	userPriorities  map[string]models.Priority
	defaultPriority models.Priority
	positiveWords   []string
	negativeWords   []string
	questionWords   []string
}

// NewPreprocessor creates a new message preprocessor
func NewPreprocessor(cfg config.QueueConfig) *Preprocessor {
	keywordPatterns := make(map[models.Priority][]*regexp.Regexp)

	// Initialize with default keyword patterns
	realtimePatterns := []string{"immediate", "emergency", "asap", "right now"}
	highPatterns := []string{"urgent", "important", "priority", "critical", "soon"}

	for _, pattern := range realtimePatterns {
		regex, err := regexp.Compile("(?i)" + pattern)
		if err == nil {
			keywordPatterns[models.PriorityRealtime] = append(keywordPatterns[models.PriorityRealtime], regex)
		}
	}

	for _, pattern := range highPatterns {
		regex, err := regexp.Compile("(?i)" + pattern)
		if err == nil {
			keywordPatterns[models.PriorityHigh] = append(keywordPatterns[models.PriorityHigh], regex)
		}
	}

	return &Preprocessor{
		keywordPatterns: keywordPatterns,
		userPriorities:  make(map[string]models.Priority),
		defaultPriority: models.PriorityNormal,
		positiveWords:   []string{"good", "great", "excellent", "happy", "satisfied"},
		negativeWords:   []string{"bad", "terrible", "awful", "angry", "frustrated"},
		questionWords:   []string{"what", "how", "why", "when", "where", "who"},
	}
}

// ProcessMessage analyzes a message and sets its priority
func (p *Preprocessor) ProcessMessage(msg *models.Message) *models.Message {
	// Initialize metadata if nil
	if msg.Metadata == nil {
		msg.Metadata = make(map[string]interface{})
	}

	// If priority is already set and not default, respect it
	if msg.Priority != models.PriorityNormal && msg.Priority != 0 {
		return msg
	}

	// Check for user priority override in metadata
	if userPriorityStr, ok := msg.Metadata["user_priority"].(string); ok {
		switch strings.ToLower(userPriorityStr) {
		case "realtime":
			msg.Priority = models.PriorityRealtime
			msg.Metadata["priority_reason"] = "user_override"
		case "high":
			msg.Priority = models.PriorityHigh
			msg.Metadata["priority_reason"] = "user_override"
		case "normal":
			msg.Priority = models.PriorityNormal
			msg.Metadata["priority_reason"] = "user_override"
		case "low":
			msg.Priority = models.PriorityLow
			msg.Metadata["priority_reason"] = "user_override"
		}
	} else if userPriority, exists := p.userPriorities[msg.UserID]; exists {
		// Check if user has a default priority
		msg.Priority = userPriority
		msg.Metadata["priority_reason"] = "user_default"
	} else {
		// Determine priority based on content analysis
		originalPriority := msg.Priority
		msg.Priority = p.analyzePriority(msg)
		if msg.Priority != originalPriority {
			msg.Metadata["priority_reason"] = "content_keywords"
		}
	}

	// Perform content analysis
	p.performContentAnalysis(msg)

	// Mark message as analyzed
	msg.Metadata["analyzed"] = true

	// Set default queue name based on priority if not specified
	if msg.QueueName == "" {
		msg.QueueName = fmt.Sprint(msg.Priority)
	}

	// Set timestamps if not already set
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	msg.UpdatedAt = time.Now()

	return msg
}

// analyzePriority determines message priority based on content
func (p *Preprocessor) analyzePriority(msg *models.Message) models.Priority {
	// Extract content for analysis
	content := msg.Content
	if content == "" {
		// If no content, check metadata for priority hints
		if len(msg.Metadata) > 0 {
			metadata := msg.Metadata
			if priorityStr, ok := metadata["priority"].(string); ok {
				switch strings.ToLower(priorityStr) {
				case "realtime":
					return models.PriorityRealtime
				case "high":
					return models.PriorityHigh
				case "normal":
					return models.PriorityNormal
				case "low":
					return models.PriorityLow
				}
			}
		}
		return p.defaultPriority
	}

	// Check for keywords in each priority level
	priorityScores := make(map[models.Priority]int)

	// Check each priority level's patterns
	for priority, patterns := range p.keywordPatterns {
		for _, pattern := range patterns {
			matches := pattern.FindAllString(content, -1)
			priorityScores[priority] += len(matches)
		}
	}

	// Find the priority with the highest score
	highestScore := 0
	highestPriority := p.defaultPriority

	for priority, score := range priorityScores {
		if score > highestScore {
			highestScore = score
			highestPriority = priority
		}
	}

	// If no keywords matched, use default priority
	if highestScore == 0 {
		return p.defaultPriority
	}

	return highestPriority
}

// SetUserPriority sets a default priority for a specific user
func (p *Preprocessor) SetUserPriority(userID string, priority models.Priority) {
	p.userPriorities[userID] = priority
}

// AddKeywordPattern adds a new keyword pattern for a priority level
func (p *Preprocessor) AddKeywordPattern(priority models.Priority, pattern string) error {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}

	p.keywordPatterns[priority] = append(p.keywordPatterns[priority], regex)
	return nil
}

// SetDefaultPriority changes the default priority
func (p *Preprocessor) SetDefaultPriority(priority models.Priority) {
	p.defaultPriority = priority
}

// GetKeywordPatterns returns all keyword patterns for a priority level
func (p *Preprocessor) GetKeywordPatterns(priority models.Priority) []*regexp.Regexp {
	return p.keywordPatterns[priority]
}

// performContentAnalysis performs content analysis and adds metadata to the message
func (p *Preprocessor) performContentAnalysis(msg *models.Message) {
	content := msg.Content
	if content == "" {
		return
	}

	// Simple word count
	words := strings.Fields(content)
	msg.Metadata["word_count"] = len(words)

	// Sentiment analysis (very basic implementation)
	positive := 0
	negative := 0

	for _, word := range words {
		wordLower := strings.ToLower(word)
		for _, pos := range p.positiveWords {
			if wordLower == pos {
				positive++
			}
		}
		for _, neg := range p.negativeWords {
			if wordLower == neg {
				negative++
			}
		}
	}

	sentiment := "neutral"
	if positive > negative {
		sentiment = "positive"
	} else if negative > positive {
		sentiment = "negative"
	}
	msg.Metadata["sentiment"] = sentiment

	// Question detection
	isQuestion := false
	if strings.HasSuffix(content, "?") {
		isQuestion = true
	} else {
		for _, keyword := range p.questionWords {
			if strings.Contains(strings.ToLower(content), keyword+" ") || strings.HasPrefix(strings.ToLower(content), keyword+" ") {
				isQuestion = true
				break
			}
		}
	}
	msg.Metadata["contains_question"] = "false"
	if isQuestion {
		msg.Metadata["contains_question"] = "true"
	}
}

// AnalyzeMessageContent performs content analysis on a message
// This could be extended with more sophisticated NLP in a real implementation
func (p *Preprocessor) AnalyzeMessageContent(content string) map[string]interface{} {
	result := make(map[string]interface{})

	// Simple word count
	words := strings.Fields(content)
	result["word_count"] = len(words)

	// Sentiment analysis (very basic implementation)
	positive := 0
	negative := 0

	for _, word := range words {
		wordLower := strings.ToLower(word)
		for _, pos := range p.positiveWords {
			if wordLower == pos {
				positive++
			}
		}
		for _, neg := range p.negativeWords {
			if wordLower == neg {
				negative++
			}
		}
	}

	result["sentiment"] = "neutral"
	if positive > negative {
		result["sentiment"] = "positive"
	} else if negative > positive {
		result["sentiment"] = "negative"
	}

	// Question detection
	result["is_question"] = false
	if strings.HasSuffix(content, "?") {
		result["is_question"] = true
	} else {
		for _, keyword := range p.questionWords {
			if strings.Contains(strings.ToLower(content), keyword+" ") || strings.HasPrefix(strings.ToLower(content), keyword+" ") {
				result["is_question"] = true
				break
			}
		}
	}

	return result
}
