package pumpfun

import (
	"fmt"
	"time"

	"pump-fun-bot-go/internal/client"
	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"

	"github.com/sirupsen/logrus"
)

const CREATE_DISCRIMINATOR uint64 = 8530921459188068891

// ListenerFactory creates different types of listeners based on configuration
type ListenerFactory struct {
	wsClient    *client.WSClient
	rpcClient   *client.Client
	config      *config.Config
	logger      *logger.Logger
	listenerMap map[config.ListenerType]bool
}

// NewListenerFactory creates a new listener factory
func NewListenerFactory(
	wsClient *client.WSClient,
	rpcClient *client.Client,
	cfg *config.Config,
	logger *logger.Logger,
) *ListenerFactory {
	return &ListenerFactory{
		wsClient:    wsClient,
		rpcClient:   rpcClient,
		config:      cfg,
		logger:      logger,
		listenerMap: make(map[config.ListenerType]bool),
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
		return lf.createMultiListener()
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

	// Create base listener
	baseListener, err := lf.CreateListener(lf.config.Listener.Type)
	if err != nil {
		return nil, fmt.Errorf("failed to create base listener: %w", err)
	}

	// Apply strategy-specific optimizations
	optimizedListener, err := lf.applyStrategyOptimizations(baseListener)
	if err != nil {
		return nil, fmt.Errorf("failed to apply strategy optimizations: %w", err)
	}

	// Apply filters if configured
	filteredListener, err := lf.applyFilters(optimizedListener)
	if err != nil {
		return nil, fmt.Errorf("failed to apply filters: %w", err)
	}

	lf.logger.WithFields(logrus.Fields{
		"final_listener_type": filteredListener.GetListenerType(),
		"strategy_optimized":  true,
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

// createMultiListener creates a multi-listener that combines multiple listener types
func (lf *ListenerFactory) createMultiListener() (ListenerInterface, error) {
	lf.logger.Info("🔄 Creating multi listener...")

	var listeners []ListenerInterface

	// Create enabled sub-listeners
	if lf.config.Listener.EnableLogListener {
		logListener, err := lf.createLogListener()
		if err != nil {
			return nil, fmt.Errorf("failed to create log listener: %w", err)
		}
		listeners = append(listeners, logListener)
		lf.logger.Info("✅ Added logs listener to multi-listener")
	}

	if lf.config.Listener.EnableBlockListener {
		blockListener, err := lf.createBlockListener()
		if err != nil {
			return nil, fmt.Errorf("failed to create block listener: %w", err)
		}
		listeners = append(listeners, blockListener)
		lf.logger.Info("✅ Added blocks listener to multi-listener")
	}

	if len(listeners) == 0 {
		return nil, fmt.Errorf("no listeners enabled for multi-listener")
	}

	// Create listener config for multi-listener
	config := &ListenerConfig{
		Config:      lf.config,
		Logger:      lf.logger,
		BufferSize:  lf.config.Listener.BufferSize,
		NetworkType: lf.config.Network,
	}

	multiListener := NewMultiListener(config, listeners...)

	lf.logger.WithFields(logrus.Fields{
		"type":             "multi",
		"sub_listeners":    len(listeners),
		"logs_enabled":     lf.config.Listener.EnableLogListener,
		"blocks_enabled":   lf.config.Listener.EnableBlockListener,
		"duplicate_filter": lf.config.Listener.EnableDuplicateFilter,
	}).Info("✅ Multi listener created")

	return multiListener, nil
}

// applyStrategyOptimizations applies strategy-specific optimizations to the listener
func (lf *ListenerFactory) applyStrategyOptimizations(baseListener ListenerInterface) (ListenerInterface, error) {
	lf.logger.Info("⚡ Applying strategy optimizations...")

	// For ultra-fast mode, no additional wrapper needed - the main app handles parallel processing
	if lf.config.UltraFast.Enabled {
		lf.logger.WithFields(logrus.Fields{
			"ultra_fast":       true,
			"parallel_workers": lf.config.UltraFast.ParallelWorkers,
			"skip_validation":  lf.config.UltraFast.SkipValidation,
			"cache_blockhash":  lf.config.UltraFast.CacheBlockhash,
		}).Info("⚡ Ultra-fast mode optimizations applied")
	}

	// For YOLO mode, ensure we get maximum speed
	if lf.config.Strategy.YoloMode {
		lf.logger.Info("🎲 YOLO mode optimizations applied - maximum speed, minimal validation")
	}

	// For hold-only mode, no special listener optimizations needed
	if lf.config.Strategy.HoldOnly {
		lf.logger.Info("🤲 Hold-only mode - all tokens will be processed")
	}

	// Return the base listener - optimizations are handled at the application level
	return baseListener, nil
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

	// Apply confirmation filter for non-YOLO modes
	if !lf.config.Strategy.YoloMode && !lf.config.UltraFast.SkipValidation {
		confirmedFilter := ConfirmedFilter()
		filters = append(filters, confirmedFilter)
		lf.logger.Info("✅ Confirmation filter applied")
	}

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
		config.MultiListenerType,
	}
}

// GetRecommendedListenerType returns the recommended listener type based on strategy
func (lf *ListenerFactory) GetRecommendedListenerType() config.ListenerType {
	// For ultra-fast mode, prefer logs listener for speed
	if lf.config.UltraFast.Enabled || lf.config.Strategy.YoloMode {
		lf.logger.Info("💡 Recommended: Logs listener (optimized for speed)")
		return config.LogsListenerType
	}

	// For conservative strategies, prefer multi-listener for coverage
	if lf.config.Strategy.HoldOnly || lf.config.Strategy.Type == "holder" {
		lf.logger.Info("💡 Recommended: Multi-listener (maximum coverage)")
		return config.MultiListenerType
	}

	// Default recommendation based on network
	if lf.config.Network == "devnet" {
		lf.logger.Info("💡 Recommended: Logs listener (devnet default)")
		return config.LogsListenerType
	}

	lf.logger.Info("💡 Recommended: Multi-listener (balanced approach)")
	return config.MultiListenerType
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
	}
}
