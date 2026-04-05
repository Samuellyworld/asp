# tests for RL trading agent

import pytest
import numpy as np
from app.rl_agent import TradingRLAgent, ReplayBuffer


def make_candles(n=50, start_price=40000, trend=0.5):
    candles = []
    price = start_price
    for i in range(n):
        price = price * (1 + trend / 100 + np.random.normal(0, 0.002))
        candles.append({
            "open": price * 0.999,
            "high": price * 1.005,
            "low": price * 0.995,
            "close": price,
            "volume": 1000000 + np.random.normal(0, 100000),
            "timestamp": i,
        })
    return candles


class TestReplayBuffer:
    def test_push_and_len(self):
        buf = ReplayBuffer(capacity=100)
        assert len(buf) == 0
        buf.push(np.zeros(5), 0, 1.0, np.zeros(5), False)
        assert len(buf) == 1

    def test_capacity_limit(self):
        buf = ReplayBuffer(capacity=10)
        for i in range(20):
            buf.push(np.zeros(5), 0, 1.0, np.zeros(5), False)
        assert len(buf) == 10

    def test_sample(self):
        buf = ReplayBuffer(capacity=100)
        for i in range(50):
            buf.push(np.random.randn(5), i % 5, float(i), np.random.randn(5), False)
        states, actions, rewards, next_states, dones = buf.sample(10)
        assert states.shape == (10, 5)
        assert actions.shape == (10,)
        assert rewards.shape == (10,)
        assert next_states.shape == (10, 5)
        assert dones.shape == (10,)


class TestTradingRLAgent:
    def setup_method(self):
        self.agent = TradingRLAgent(state_size=12, epsilon=0.5)

    def test_actions_list(self):
        assert len(TradingRLAgent.ACTIONS) == 5
        assert "hold" in TradingRLAgent.ACTIONS

    def test_get_action_returns_valid(self):
        state = np.random.randn(12).astype(np.float32)
        result = self.agent.get_action(state)
        assert result["action"] in TradingRLAgent.ACTIONS
        assert 0 <= result["action_idx"] < 5

    def test_get_action_no_explore(self):
        state = np.random.randn(12).astype(np.float32)
        result = self.agent.get_action(state, explore=False)
        # without torch, agent always returns exploring=True
        if self.agent.policy_net is not None:
            assert result["exploring"] is False
        else:
            assert result["exploring"] is True

    def test_extract_state_shape(self):
        candles = make_candles(30)
        state = self.agent._extract_state(candles, 25, 10000, 0, 0, 10000)
        assert state.shape == (12,)
        assert not np.isnan(state).any()

    def test_extract_state_with_position(self):
        candles = make_candles(30)
        price = candles[-1]["close"]
        state = self.agent._extract_state(candles, 29, 5000, 0.1, price * 0.95, 10000)
        assert state.shape == (12,)

    def test_execute_action_hold(self):
        bal, pos, entry = self.agent._execute_action(0, 10000, 0, 0, 40000)
        assert bal == 10000
        assert pos == 0

    def test_execute_action_buy_small(self):
        bal, pos, entry = self.agent._execute_action(1, 10000, 0, 0, 40000)
        assert bal == 7500  # 25% used
        assert pos > 0
        assert entry == 40000

    def test_execute_action_buy_large(self):
        bal, pos, entry = self.agent._execute_action(2, 10000, 0, 0, 40000)
        assert bal == 5000  # 50% used
        assert pos > 0

    def test_execute_action_sell_all(self):
        bal, pos, entry = self.agent._execute_action(4, 5000, 0.1, 40000, 42000)
        assert pos == 0
        assert bal == 5000 + 0.1 * 42000

    def test_execute_action_sell_small(self):
        bal, pos, entry = self.agent._execute_action(3, 5000, 1.0, 40000, 42000)
        assert pos == 0.75
        assert bal > 5000

    def test_compute_reward_positive(self):
        reward = self.agent._compute_reward(0.1, 40000, 41000, 5000, 10000)
        assert reward > 0

    def test_compute_reward_negative(self):
        reward = self.agent._compute_reward(0.1, 40000, 39000, 5000, 10000)
        assert reward < 0

    def test_compute_reward_no_position(self):
        reward = self.agent._compute_reward(0, 40000, 41000, 10000, 10000)
        assert reward < 0  # opportunity cost

    def test_compute_reward_drawdown_penalty(self):
        # large drawdown scenario
        reward = self.agent._compute_reward(0, 40000, 41000, 5000, 10000)
        assert reward < -0.1  # should be penalized

    def test_train_step_returns_none_without_data(self):
        loss = self.agent.train_step()
        assert loss is None

    def test_train_from_backtest_insufficient(self):
        candles = make_candles(10)
        result = self.agent.train_from_backtest(candles, episodes=1)
        assert result["success"] is False

    def test_train_from_backtest_basic(self):
        candles = make_candles(100, trend=0.5)
        result = self.agent.train_from_backtest(candles, episodes=3, initial_balance=10000)
        if self.agent.policy_net is not None:
            assert result["success"] is True
            assert result["episodes"] == 3
            assert "avg_reward_last_20" in result
        else:
            # without torch, training returns failure
            assert result["success"] is False

    def test_epsilon_decays(self):
        if self.agent.policy_net is None:
            pytest.skip("pytorch not available")
        initial_eps = self.agent.epsilon
        candles = make_candles(100, trend=0.5)
        self.agent.train_from_backtest(candles, episodes=5)
        assert self.agent.epsilon < initial_eps

    def test_update_target(self):
        # should not raise
        self.agent.update_target()
