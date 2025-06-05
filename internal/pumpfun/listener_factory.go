package pumpfun

import (
	"fmt"
	"pump-fun-bot-go/internal/client"
	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"time"

	"github.com/sirupsen/logrus"
)

const CREATE_DISCRIMINATOR uint64 = 8530921459188068891

// ListenerFactory creates different types of listeners based on configuration
type ListenerFactory struct {
	wsClient  *client.WSClient
	rpcClient *client.Client
	config    *config.Config
	logger    *logger.Logger
}

// NewListenerFactory creates a new listener factory
func NewListenerFactory(
	wsClient *client.WSClient,
	rpcClient *client.Client,
	cfg *config.Config,
	logger *logger.Logger,
) *ListenerFactory {
	return &ListenerFactory{
		wsClient:  wsClient,
		rpcClient: rpcClient,
		config:    cfg,
		logger:    logger,
	}
}

// CreateListener creates a listener based on type
func (lf *ListenerFactory) CreateListener(listenerType config.ListenerType) (ListenerInterface, error) {
	lf.logger.WithField("type", listenerType).Info("🏭 Creating listener...")

	switch listenerType {
	case config.LogsListenerType:
		return lf.createLogListener()
	case config.BlocksListenerType:
		return lf.createBlockListener()
	case config.MultiListenerType:
		// Для multi-listener выбираем самый подходящий тип
		lf.logger.Info("🔄 Multi-listener requested, selecting best single listener")
		if lf.config.UltraFast.Enabled || lf.config.Strategy.YoloMode {
			lf.logger.Info("⚡ Using logs listener for speed")
			return lf.createLogListener()
		} else {
			lf.logger.Info("📦 Using blocks listener for reliability")
			return lf.createBlockListener()
		}
	default:
		return nil, fmt.Errorf("unsupported listener type: %s", listenerType)
	}
}

// CreateStrategyListener creates a listener based on current configuration and strategy
func (lf *ListenerFactory) CreateStrategyListener() (ListenerInterface, error) {
	lf.logger.Info("🎯 Creating strategy-optimized listener...")

	// Log the strategy being used
	lf.logger.WithFields(logrus.Fields{
		"listener_type":      lf.config.Listener.Type,
		"strategy_type":      lf.config.Strategy.Type,
		"yolo_mode":          lf.config.Strategy.YoloMode,
		"hold_only":          lf.config.Strategy.HoldOnly,
		"ultra_fast_enabled": lf.config.UltraFast.Enabled,
		"parallel_workers":   lf.config.UltraFast.ParallelWorkers,
		"skip_validation":    lf.config.UltraFast.SkipValidation,
	}).Info("🎯 Strategy configuration")

	// Create base listener based on strategy
	var baseListener ListenerInterface
	var err error

	// Choose listener type based on strategy
	if lf.config.Listener.Type == config.BlocksListenerType {
		lf.logger.Info("📦 Using blocks listener as configured")
		baseListener, err = lf.createBlockListener()
	} else {
		// Default to logs
		lf.logger.Info("📋 Using logs listener (default)")
		baseListener, err = lf.createLogListener()
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create base listener: %w", err)
	}

	// Apply filters if configured
	filteredListener, err := lf.applyFilters(baseListener)
	if err != nil {
		return nil, fmt.Errorf("failed to apply filters: %w", err)
	}

	lf.logger.WithFields(logrus.Fields{
		"final_listener_type": filteredListener.GetListenerType(),
		"filters_applied":     lf.hasFiltersConfigured(),
	}).Info("✅ Strategy listener created successfully")

	return filteredListener, nil
}

// createLogListener creates a logs-based listener
func (lf *ListenerFactory) createLogListener() (ListenerInterface, error) {
	lf.logger.Info("📋 Creating logs listener...")

	listener := NewLogListener(lf.wsClient, lf.rpcClient, lf.config, lf.logger)

	lf.logger.WithFields(logrus.Fields{
		"type":        "logs",
		"buffer_size": lf.config.Listener.BufferSize,
		"network":     lf.config.Network,
		"program_id":  "pump_fun",
	}).Info("✅ Logs listener created")

	return listener, nil
}

// createBlockListener creates a blocks-based listener
func (lf *ListenerFactory) createBlockListener() (ListenerInterface, error) {
	lf.logger.Info("📦 Creating blocks listener...")

	listener := NewBlockListener(lf.wsClient, lf.rpcClient, lf.config, lf.logger)

	lf.logger.WithFields(logrus.Fields{
		"type":        "blocks",
		"buffer_size": lf.config.Listener.BufferSize,
		"network":     lf.config.Network,
		"confirmed":   true,
	}).Info("✅ Blocks listener created")

	return listener, nil
}

// applyFilters applies configured filters to the listener
func (lf *ListenerFactory) applyFilters(baseListener ListenerInterface) (ListenerInterface, error) {
	var filters []TokenFilter

	// Apply name pattern filters
	if lf.config.Strategy.FilterByName && len(lf.config.Strategy.NamePatterns) > 0 {
		nameFilter := NameFilter(lf.config.Strategy.NamePatterns)
		filters = append(filters, nameFilter)
		lf.logger.WithField("patterns", lf.config.Strategy.NamePatterns).Info("🏷️ Name filter applied")
	}

	// Apply creator filters
	if lf.config.Strategy.FilterByCreator && len(lf.config.Strategy.AllowedCreators) > 0 {
		creatorFilter := CreatorFilter(lf.config.Strategy.AllowedCreators)
		filters = append(filters, creatorFilter)
		lf.logger.WithField("creators", lf.config.Strategy.AllowedCreators).Info("👤 Creator filter applied")
	}

	// Apply freshness filter for ultra-fast mode
	if lf.config.UltraFast.Enabled && lf.config.Trading.MaxTokenAgeMs > 0 {
		maxAge := time.Duration(lf.config.Trading.MaxTokenAgeMs) * time.Millisecond
		freshnessFilter := FreshnessFilter(maxAge)
		filters = append(filters, freshnessFilter)
		lf.logger.WithField("max_age_ms", lf.config.Trading.MaxTokenAgeMs).Info("⏰ Freshness filter applied")
	}

	//Apply confirmation filter for non-YOLO modes
	//if !lf.config.Strategy.YoloMode && !lf.config.UltraFast.SkipValidation {
	//	confirmedFilter := ConfirmedFilter()
	//	filters = append(filters, confirmedFilter)
	//	lf.logger.Info("✅ Confirmation filter applied")
	//}

	// If no filters, return base listener
	if len(filters) == 0 {
		lf.logger.Info("🔓 No filters configured - all tokens will be processed")
		return baseListener, nil
	}

	// Create filtered listener
	config := &ListenerConfig{
		Config:      lf.config,
		Logger:      lf.logger,
		BufferSize:  lf.config.Listener.BufferSize,
		NetworkType: lf.config.Network,
	}

	filteredListener := NewFilteredListener(baseListener, config, filters...)

	lf.logger.WithFields(logrus.Fields{
		"filters_count":       len(filters),
		"name_filter":         lf.config.Strategy.FilterByName,
		"creator_filter":      lf.config.Strategy.FilterByCreator,
		"freshness_filter":    lf.config.Trading.MaxTokenAgeMs > 0,
		"confirmation_filter": !lf.config.Strategy.YoloMode,
	}).Info("🔍 Filtered listener created")

	return filteredListener, nil
}

// hasFiltersConfigured checks if any filters are configured
func (lf *ListenerFactory) hasFiltersConfigured() bool {
	return (lf.config.Strategy.FilterByName && len(lf.config.Strategy.NamePatterns) > 0) ||
		(lf.config.Strategy.FilterByCreator && len(lf.config.Strategy.AllowedCreators) > 0) ||
		(lf.config.UltraFast.Enabled && lf.config.Trading.MaxTokenAgeMs > 0) ||
		(!lf.config.Strategy.YoloMode && !lf.config.UltraFast.SkipValidation)
}

// GetSupportedListenerTypes returns all supported listener types
func (lf *ListenerFactory) GetSupportedListenerTypes() []config.ListenerType {
	return []config.ListenerType{
		config.LogsListenerType,
		config.BlocksListenerType,
		config.MultiListenerType, // Теперь это просто алиас для автовыбора
	}
}

// GetRecommendedListenerType returns the recommended listener type based on strategy
func (lf *ListenerFactory) GetRecommendedListenerType() config.ListenerType {
	// For ultra-fast mode, prefer logs listener for speed
	if lf.config.UltraFast.Enabled || lf.config.Strategy.YoloMode {
		lf.logger.Info("💡 Recommended: Logs listener (optimized for speed)")
		return config.LogsListenerType
	}

	// For conservative strategies, prefer blocks listener for reliability
	if lf.config.Strategy.HoldOnly || lf.config.Strategy.Type == "holder" {
		lf.logger.Info("💡 Recommended: Blocks listener (higher reliability)")
		return config.BlocksListenerType
	}

	// Default recommendation based on network
	if lf.config.Network == "devnet" {
		lf.logger.Info("💡 Recommended: Logs listener (devnet default)")
		return config.LogsListenerType
	}

	lf.logger.Info("💡 Recommended: Logs listener (balanced approach)")
	return config.LogsListenerType
}

// CreateOptimalListener creates the optimal listener for current configuration
func (lf *ListenerFactory) CreateOptimalListener() (ListenerInterface, error) {
	// Use configured type if specified, otherwise use recommended
	listenerType := lf.config.Listener.Type
	if listenerType == "" {
		listenerType = lf.GetRecommendedListenerType()
		lf.logger.WithField("auto_selected_type", listenerType).Info("🤖 Auto-selected optimal listener type")
	}

	return lf.CreateListener(listenerType)
}

// GetFactoryStats returns factory statistics and configuration
func (lf *ListenerFactory) GetFactoryStats() map[string]interface{} {
	return map[string]interface{}{
		"factory_type":         "listener_factory",
		"configured_type":      lf.config.Listener.Type,
		"recommended_type":     lf.GetRecommendedListenerType(),
		"supported_types":      lf.GetSupportedListenerTypes(),
		"buffer_size":          lf.config.Listener.BufferSize,
		"parallel_processing":  lf.config.Listener.ParallelProcessing,
		"worker_count":         lf.config.Listener.WorkerCount,
		"duplicate_filter":     lf.config.Listener.EnableDuplicateFilter,
		"duplicate_filter_ttl": lf.config.Listener.DuplicateFilterTTL,
		"filters_configured":   lf.hasFiltersConfigured(),
		"ultra_fast_enabled":   lf.config.UltraFast.Enabled,
		"strategy_type":        lf.config.Strategy.Type,
		"yolo_mode":            lf.config.Strategy.YoloMode,
		"hold_only":            lf.config.Strategy.HoldOnly,
		"network":              lf.config.Network,
		"simplified_factory":   true, // Указываем что это упрощенная версия
	}
}
