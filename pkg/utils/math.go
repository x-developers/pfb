package utils

import (
	"math"
	"math/big"

	"pump-fun-bot-go/internal/config"
)

// ConvertSOLToLamports converts SOL to lamports
func ConvertSOLToLamports(sol float64) uint64 {
	return uint64(sol * config.LamportsPerSol)
}

// ConvertLamportsToSOL converts lamports to SOL
func ConvertLamportsToSOL(lamports uint64) float64 {
	return float64(lamports) / config.LamportsPerSol
}

// SafeDiv performs safe division avoiding division by zero
func SafeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// SafeU64Div performs safe u64 division
func SafeU64Div(a, b uint64) uint64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// CalculatePercentageChange calculates percentage change between two values
func CalculatePercentageChange(oldValue, newValue float64) float64 {
	if oldValue == 0 {
		return 0
	}
	return ((newValue - oldValue) / oldValue) * 100
}

// CalculateSlippage calculates slippage percentage
func CalculateSlippage(expected, actual float64) float64 {
	if expected == 0 {
		return 0
	}
	return math.Abs((actual-expected)/expected) * 100
}

// ApplySlippage applies slippage tolerance to a value
func ApplySlippage(amount float64, slippageBP int, increase bool) float64 {
	slippagePercent := float64(slippageBP) / 10000 // Convert basis points to decimal

	if increase {
		return amount * (1 + slippagePercent)
	}
	return amount * (1 - slippagePercent)
}

// BasisPointsToPercent converts basis points to percentage
func BasisPointsToPercent(bp int) float64 {
	return float64(bp) / 100
}

// PercentToBasisPoints converts percentage to basis points
func PercentToBasisPoints(percent float64) int {
	return int(percent * 100)
}

// CompoundInterest calculates compound interest
func CompoundInterest(principal, rate float64, periods int) float64 {
	return principal * math.Pow(1+rate, float64(periods))
}

// CalculateROI calculates Return on Investment
func CalculateROI(initialInvestment, finalValue float64) float64 {
	if initialInvestment == 0 {
		return 0
	}
	return ((finalValue - initialInvestment) / initialInvestment) * 100
}

// WeightedAverage calculates weighted average
func WeightedAverage(values []float64, weights []float64) float64 {
	if len(values) != len(weights) || len(values) == 0 {
		return 0
	}

	var sum, weightSum float64
	for i, value := range values {
		sum += value * weights[i]
		weightSum += weights[i]
	}

	if weightSum == 0 {
		return 0
	}
	return sum / weightSum
}

// Min returns the minimum of two uint64 values
func MinU64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

// Max returns the maximum of two uint64 values
func MaxU64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

// Min returns the minimum of two float64 values
func MinF64(a, b float64) float64 {
	return math.Min(a, b)
}

// Max returns the maximum of two float64 values
func MaxF64(a, b float64) float64 {
	return math.Max(a, b)
}

// Clamp clamps a value between min and max
func ClampF64(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// ClampU64 clamps a uint64 value between min and max
func ClampU64(value, min, max uint64) uint64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// Sqrt calculates square root using big.Float for precision
func Sqrt(x float64) float64 {
	if x < 0 {
		return 0
	}
	return math.Sqrt(x)
}

// SqrtU64 calculates square root of uint64
func SqrtU64(x uint64) uint64 {
	if x == 0 {
		return 0
	}
	return uint64(math.Sqrt(float64(x)))
}

// PowerU64 calculates a^b for uint64 with overflow protection
func PowerU64(base, exp uint64) uint64 {
	if exp == 0 {
		return 1
	}
	if base == 0 {
		return 0
	}

	result := uint64(1)
	for exp > 0 {
		if exp%2 == 1 {
			// Check for overflow
			if result > math.MaxUint64/base {
				return math.MaxUint64 // Return max value on overflow
			}
			result *= base
		}

		// Check for overflow in base squaring
		if base > math.MaxUint64/base {
			if exp > 1 {
				return math.MaxUint64
			}
		}
		base *= base
		exp /= 2
	}

	return result
}

// IsWithinTolerance checks if two values are within tolerance
func IsWithinTolerance(value1, value2, tolerance float64) bool {
	return math.Abs(value1-value2) <= tolerance
}

// RoundTo rounds a float64 to specified decimal places
func RoundTo(value float64, decimals int) float64 {
	multiplier := math.Pow(10, float64(decimals))
	return math.Round(value*multiplier) / multiplier
}

// CeilTo rounds up to specified decimal places
func CeilTo(value float64, decimals int) float64 {
	multiplier := math.Pow(10, float64(decimals))
	return math.Ceil(value*multiplier) / multiplier
}

// FloorTo rounds down to specified decimal places
func FloorTo(value float64, decimals int) float64 {
	multiplier := math.Pow(10, float64(decimals))
	return math.Floor(value*multiplier) / multiplier
}

// BondingCurve calculations

// ConstantProductPrice calculates price using constant product formula (x * y = k)
func ConstantProductPrice(reserveX, reserveY uint64) float64 {
	if reserveX == 0 {
		return 0
	}
	return float64(reserveY) / float64(reserveX)
}

// ConstantProductBuy calculates cost for buying tokens using constant product
func ConstantProductBuy(reserveX, reserveY, tokenAmount uint64) uint64 {
	if reserveX == 0 || tokenAmount >= reserveX {
		return 0
	}

	// Using big.Float for precision in calculations
	x := new(big.Float).SetUint64(reserveX)
	y := new(big.Float).SetUint64(reserveY)
	amount := new(big.Float).SetUint64(tokenAmount)

	// k = x * y (constant product)
	k := new(big.Float).Mul(x, y)

	// newX = x - tokenAmount
	newX := new(big.Float).Sub(x, amount)

	// newY = k / newX
	newY := new(big.Float).Quo(k, newX)

	// cost = newY - y
	cost := new(big.Float).Sub(newY, y)

	// Convert back to uint64
	costU64, _ := cost.Uint64()
	return costU64
}

// ConstantProductSell calculates output for selling tokens using constant product
func ConstantProductSell(reserveX, reserveY, tokenAmount uint64) uint64 {
	if reserveX == 0 || reserveY == 0 {
		return 0
	}

	// Using big.Float for precision
	x := new(big.Float).SetUint64(reserveX)
	y := new(big.Float).SetUint64(reserveY)
	amount := new(big.Float).SetUint64(tokenAmount)

	// k = x * y
	k := new(big.Float).Mul(x, y)

	// newX = x + tokenAmount
	newX := new(big.Float).Add(x, amount)

	// newY = k / newX
	newY := new(big.Float).Quo(k, newX)

	// output = y - newY
	output := new(big.Float).Sub(y, newY)

	// Convert back to uint64
	outputU64, _ := output.Uint64()
	return outputU64
}

// LinearBondingCurvePrice calculates price for linear bonding curve
func LinearBondingCurvePrice(supply, slope uint64) float64 {
	return float64(supply) * float64(slope)
}

// ExponentialBondingCurvePrice calculates price for exponential bonding curve
func ExponentialBondingCurvePrice(supply uint64, base, exponent float64) float64 {
	return base * math.Pow(float64(supply), exponent)
}

// Statistics functions

// Average calculates arithmetic mean
func Average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sum := 0.0
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

// Median calculates median value
func Median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Simple implementation - in production you'd want to sort the slice
	// This is a basic approximation
	return Average(values)
}

// StandardDeviation calculates standard deviation
func StandardDeviation(values []float64) float64 {
	if len(values) <= 1 {
		return 0
	}

	mean := Average(values)
	sumSquaredDiffs := 0.0

	for _, value := range values {
		diff := value - mean
		sumSquaredDiffs += diff * diff
	}

	variance := sumSquaredDiffs / float64(len(values)-1)
	return math.Sqrt(variance)
}

// MovingAverage calculates simple moving average
func MovingAverage(values []float64, period int) []float64 {
	if len(values) < period || period <= 0 {
		return make([]float64, 0)
	}

	result := make([]float64, len(values)-period+1)

	for i := 0; i <= len(values)-period; i++ {
		sum := 0.0
		for j := 0; j < period; j++ {
			sum += values[i+j]
		}
		result[i] = sum / float64(period)
	}

	return result
}

// ExponentialMovingAverage calculates exponential moving average
func ExponentialMovingAverage(values []float64, alpha float64) []float64 {
	if len(values) == 0 || alpha <= 0 || alpha > 1 {
		return make([]float64, 0)
	}

	result := make([]float64, len(values))
	result[0] = values[0]

	for i := 1; i < len(values); i++ {
		result[i] = alpha*values[i] + (1-alpha)*result[i-1]
	}

	return result
}

// Volatility calculates price volatility (standard deviation of returns)
func Volatility(prices []float64) float64 {
	if len(prices) < 2 {
		return 0
	}

	returns := make([]float64, len(prices)-1)
	for i := 1; i < len(prices); i++ {
		if prices[i-1] != 0 {
			returns[i-1] = (prices[i] - prices[i-1]) / prices[i-1]
		}
	}

	return StandardDeviation(returns)
}

// SharpeRatio calculates Sharpe ratio (return/risk ratio)
func SharpeRatio(returns []float64, riskFreeRate float64) float64 {
	if len(returns) == 0 {
		return 0
	}

	avgReturn := Average(returns)
	volatility := StandardDeviation(returns)

	if volatility == 0 {
		return 0
	}

	return (avgReturn - riskFreeRate) / volatility
}

// MaxDrawdown calculates maximum drawdown from peak
func MaxDrawdown(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	peak := values[0]
	maxDrawdown := 0.0

	for _, value := range values {
		if value > peak {
			peak = value
		}

		drawdown := (peak - value) / peak
		if drawdown > maxDrawdown {
			maxDrawdown = drawdown
		}
	}

	return maxDrawdown * 100 // Return as percentage
}

// Value at Risk (VaR) calculation using historical method
func ValueAtRisk(returns []float64, confidenceLevel float64) float64 {
	if len(returns) == 0 || confidenceLevel <= 0 || confidenceLevel >= 1 {
		return 0
	}

	// Simple percentile-based VaR
	// In production, you'd sort the returns and find the appropriate percentile
	avgReturn := Average(returns)
	stdDev := StandardDeviation(returns)

	// Assuming normal distribution (simplified)
	// For 95% confidence level, z-score is approximately 1.645
	var zScore float64
	switch {
	case confidenceLevel >= 0.99:
		zScore = 2.33
	case confidenceLevel >= 0.95:
		zScore = 1.645
	case confidenceLevel >= 0.90:
		zScore = 1.28
	default:
		zScore = 1.0
	}

	return math.Abs(avgReturn - zScore*stdDev)
}

// Kelly Criterion calculates optimal bet size
func KellyCriterion(winProbability, winPayoff, lossProbability, lossAmount float64) float64 {
	if lossProbability == 0 || lossAmount == 0 {
		return 0
	}

	// Kelly % = (bp - q) / b
	// where b = win payoff ratio, p = win probability, q = loss probability
	b := winPayoff / lossAmount
	p := winProbability
	q := lossProbability

	kelly := (b*p - q) / b

	// Clamp between 0 and 1 (never bet more than 100% or go negative)
	return ClampF64(kelly, 0, 1)
}

// Risk-adjusted return calculations

// CalmarRatio calculates Calmar ratio (annual return / max drawdown)
func CalmarRatio(annualReturn, maxDrawdown float64) float64 {
	if maxDrawdown == 0 {
		return 0
	}
	return annualReturn / maxDrawdown
}

// SortinoRatio calculates Sortino ratio (return / downside deviation)
func SortinoRatio(returns []float64, targetReturn float64) float64 {
	if len(returns) == 0 {
		return 0
	}

	avgReturn := Average(returns)

	// Calculate downside deviation
	downsideSquaredDiffs := 0.0
	downsideCount := 0

	for _, ret := range returns {
		if ret < targetReturn {
			diff := ret - targetReturn
			downsideSquaredDiffs += diff * diff
			downsideCount++
		}
	}

	if downsideCount == 0 {
		return math.Inf(1) // Infinite Sortino ratio (no downside)
	}

	downsideDeviation := math.Sqrt(downsideSquaredDiffs / float64(downsideCount))

	if downsideDeviation == 0 {
		return 0
	}

	return (avgReturn - targetReturn) / downsideDeviation
}

// TreynorRatio calculates Treynor ratio (return / beta)
func TreynorRatio(portfolioReturn, riskFreeRate, beta float64) float64 {
	if beta == 0 {
		return 0
	}
	return (portfolioReturn - riskFreeRate) / beta
}

// Information Ratio calculates Information ratio (active return / tracking error)
func InformationRatio(portfolioReturns, benchmarkReturns []float64) float64 {
	if len(portfolioReturns) != len(benchmarkReturns) || len(portfolioReturns) == 0 {
		return 0
	}

	activeReturns := make([]float64, len(portfolioReturns))
	for i := range portfolioReturns {
		activeReturns[i] = portfolioReturns[i] - benchmarkReturns[i]
	}

	avgActiveReturn := Average(activeReturns)
	trackingError := StandardDeviation(activeReturns)

	if trackingError == 0 {
		return 0
	}

	return avgActiveReturn / trackingError
}
