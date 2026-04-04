// performance metrics for backtesting results.
package backtest

import (
	"math"
	"sort"
	"time"
)

// Metrics holds computed performance statistics.
type Metrics struct {
	// returns
	TotalReturn     float64 // total P&L in USD
	TotalReturnPct  float64 // total return as percentage
	AnnualizedReturn float64 // CAGR

	// trade stats
	TotalTrades     int
	WinningTrades   int
	LosingTrades    int
	WinRate         float64 // 0-100
	ProfitFactor    float64 // gross profit / gross loss
	AvgWin          float64
	AvgLoss         float64
	AvgWinPct       float64
	AvgLossPct      float64
	LargestWin      float64
	LargestLoss     float64
	AvgBarsHeld     float64

	// streaks
	MaxConsecWins   int
	MaxConsecLosses int

	// risk
	MaxDrawdown     float64 // as percentage
	MaxDrawdownUSD  float64
	SharpeRatio     float64 // annualized, assuming risk-free rate = 0
	SortinoRatio    float64
	CalmarRatio     float64 // annualized return / max drawdown

	// fees
	TotalFees float64

	// timing
	StartDate time.Time
	EndDate   time.Time
	Duration  time.Duration
}

// ComputeMetrics calculates all performance metrics from a backtest result.
func ComputeMetrics(result *Result) *Metrics {
	m := &Metrics{
		TotalTrades: len(result.Trades),
		TotalReturn: result.FinalEquity - result.Config.InitialCapital,
	}

	if result.Config.InitialCapital > 0 {
		m.TotalReturnPct = m.TotalReturn / result.Config.InitialCapital * 100
	}

	if len(result.EquityCurve) > 0 {
		m.StartDate = result.EquityCurve[0].Time
		m.EndDate = result.EquityCurve[len(result.EquityCurve)-1].Time
		m.Duration = m.EndDate.Sub(m.StartDate)
	}

	// annualized return (CAGR)
	years := m.Duration.Hours() / (365.25 * 24)
	if years > 0 && result.Config.InitialCapital > 0 && result.FinalEquity > 0 {
		m.AnnualizedReturn = (math.Pow(result.FinalEquity/result.Config.InitialCapital, 1/years) - 1) * 100
	}

	if len(result.Trades) == 0 {
		return m
	}

	// classify trades
	var grossProfit, grossLoss float64
	var totalBars int
	var winPcts, lossPcts []float64
	consecWins, consecLosses := 0, 0

	for _, t := range result.Trades {
		m.TotalFees += t.EntryFee + t.ExitFee
		totalBars += t.Bars

		if t.PnL > 0 {
			m.WinningTrades++
			grossProfit += t.PnL
			winPcts = append(winPcts, t.PnLPercent)

			consecWins++
			consecLosses = 0
			if consecWins > m.MaxConsecWins {
				m.MaxConsecWins = consecWins
			}

			if t.PnL > m.LargestWin {
				m.LargestWin = t.PnL
			}
		} else if t.PnL < 0 {
			m.LosingTrades++
			grossLoss += math.Abs(t.PnL)
			lossPcts = append(lossPcts, t.PnLPercent)

			consecLosses++
			consecWins = 0
			if consecLosses > m.MaxConsecLosses {
				m.MaxConsecLosses = consecLosses
			}

			if t.PnL < m.LargestLoss {
				m.LargestLoss = t.PnL
			}
		}
	}

	if m.TotalTrades > 0 {
		m.WinRate = float64(m.WinningTrades) / float64(m.TotalTrades) * 100
		m.AvgBarsHeld = float64(totalBars) / float64(m.TotalTrades)
	}
	if m.WinningTrades > 0 {
		m.AvgWin = grossProfit / float64(m.WinningTrades)
		m.AvgWinPct = avg(winPcts)
	}
	if m.LosingTrades > 0 {
		m.AvgLoss = -grossLoss / float64(m.LosingTrades)
		m.AvgLossPct = avg(lossPcts)
	}
	if grossLoss > 0 {
		m.ProfitFactor = grossProfit / grossLoss
	}

	// max drawdown from equity curve
	m.MaxDrawdown, m.MaxDrawdownUSD = maxDrawdown(result.EquityCurve)

	// sharpe and sortino from trade returns
	m.SharpeRatio = sharpeRatio(result.Trades, years)
	m.SortinoRatio = sortinoRatio(result.Trades, years)

	if m.MaxDrawdown > 0 {
		m.CalmarRatio = m.AnnualizedReturn / m.MaxDrawdown
	}

	return m
}

func maxDrawdown(equity []EquityPoint) (pct float64, usd float64) {
	if len(equity) == 0 {
		return 0, 0
	}

	peak := equity[0].Equity
	for _, ep := range equity {
		if ep.Equity > peak {
			peak = ep.Equity
		}
		drawdown := peak - ep.Equity
		if drawdown > usd {
			usd = drawdown
		}
		if peak > 0 {
			ddPct := drawdown / peak * 100
			if ddPct > pct {
				pct = ddPct
			}
		}
	}
	return pct, usd
}

func sharpeRatio(trades []Trade, years float64) float64 {
	if len(trades) < 2 || years <= 0 {
		return 0
	}

	returns := make([]float64, len(trades))
	for i, t := range trades {
		returns[i] = t.PnLPercent / 100
	}

	mean := avg(returns)
	stdDev := stddev(returns, mean)
	if stdDev == 0 {
		return 0
	}

	tradesPerYear := float64(len(trades)) / years
	return (mean / stdDev) * math.Sqrt(tradesPerYear)
}

func sortinoRatio(trades []Trade, years float64) float64 {
	if len(trades) < 2 || years <= 0 {
		return 0
	}

	returns := make([]float64, len(trades))
	for i, t := range trades {
		returns[i] = t.PnLPercent / 100
	}

	mean := avg(returns)

	// downside deviation: only negative returns
	var sumSqNeg float64
	var count int
	for _, r := range returns {
		if r < 0 {
			sumSqNeg += r * r
			count++
		}
	}
	if count == 0 {
		return 0
	}

	downsideDev := math.Sqrt(sumSqNeg / float64(len(returns)))
	if downsideDev == 0 {
		return 0
	}

	tradesPerYear := float64(len(trades)) / years
	return (mean / downsideDev) * math.Sqrt(tradesPerYear)
}

func avg(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func stddev(vals []float64, mean float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	sumSq := 0.0
	for _, v := range vals {
		d := v - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(vals)-1))
}

// TradesByPnL returns trades sorted by P&L descending.
func TradesByPnL(trades []Trade) []Trade {
	sorted := make([]Trade, len(trades))
	copy(sorted, trades)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].PnL > sorted[j].PnL
	})
	return sorted
}
