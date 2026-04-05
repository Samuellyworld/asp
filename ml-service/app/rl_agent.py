# reinforcement learning agent for trading decisions
# learns optimal position sizing and entry/exit from backtest results

import logging
import numpy as np
from typing import Optional
from collections import deque

logger = logging.getLogger(__name__)

try:
    import torch
    import torch.nn as nn
    import torch.optim as optim
    TORCH_AVAILABLE = True
except ImportError:
    TORCH_AVAILABLE = False

    class _ModuleStub:
        pass

    class _NNStub:
        Module = _ModuleStub
        Linear = None
        ReLU = None
        Dropout = None
        Sequential = None
        MSELoss = None

    nn = _NNStub()


class DQNetwork(nn.Module):
    """deep Q-network for trading actions"""

    def __init__(self, state_size: int = 12, action_size: int = 5, hidden: int = 128):
        super().__init__()
        self.net = nn.Sequential(
            nn.Linear(state_size, hidden),
            nn.ReLU(),
            nn.Dropout(0.1),
            nn.Linear(hidden, hidden),
            nn.ReLU(),
            nn.Dropout(0.1),
            nn.Linear(hidden, action_size),
        )

    def forward(self, x):
        return self.net(x)


class ReplayBuffer:
    """experience replay buffer for DQN training"""

    def __init__(self, capacity: int = 10000):
        self.buffer = deque(maxlen=capacity)

    def push(self, state, action, reward, next_state, done):
        self.buffer.append((state, action, reward, next_state, done))

    def sample(self, batch_size: int):
        indices = np.random.choice(len(self.buffer), batch_size, replace=False)
        batch = [self.buffer[i] for i in indices]
        states, actions, rewards, next_states, dones = zip(*batch)
        return (
            np.array(states),
            np.array(actions),
            np.array(rewards, dtype=np.float32),
            np.array(next_states),
            np.array(dones, dtype=np.float32),
        )

    def __len__(self):
        return len(self.buffer)


class TradingRLAgent:
    """reinforcement learning agent that learns from backtest results.
    
    actions:
        0 = hold (do nothing)
        1 = buy small (25% of available)
        2 = buy large (50% of available)
        3 = sell small (25% of position)
        4 = sell all (100% of position)
    
    state features:
        - price returns (last 5 bars)
        - current position ratio
        - unrealized pnl
        - volatility (ATR-like)
        - momentum
        - volume ratio
        - rsi-like indicator
    """

    ACTIONS = ["hold", "buy_small", "buy_large", "sell_small", "sell_all"]

    def __init__(self, state_size: int = 12, learning_rate: float = 0.001,
                 gamma: float = 0.99, epsilon: float = 1.0,
                 epsilon_decay: float = 0.995, epsilon_min: float = 0.01,
                 batch_size: int = 64):
        self.state_size = state_size
        self.action_size = len(self.ACTIONS)
        self.gamma = gamma
        self.epsilon = epsilon
        self.epsilon_decay = epsilon_decay
        self.epsilon_min = epsilon_min
        self.batch_size = batch_size

        self.replay_buffer = ReplayBuffer()
        self.training_history: list[dict] = []

        if TORCH_AVAILABLE:
            self.policy_net = DQNetwork(state_size, self.action_size)
            self.target_net = DQNetwork(state_size, self.action_size)
            self.target_net.load_state_dict(self.policy_net.state_dict())
            self.optimizer = optim.Adam(self.policy_net.parameters(), lr=learning_rate)
        else:
            self.policy_net = None
            self.target_net = None
            self.optimizer = None

    def get_action(self, state: np.ndarray, explore: bool = True) -> dict:
        """selects an action given current state"""
        if self.policy_net is None:
            return {"action": "hold", "action_idx": 0, "q_values": [], "exploring": True}

        # epsilon-greedy
        if explore and np.random.random() < self.epsilon:
            action_idx = np.random.randint(self.action_size)
            exploring = True
        else:
            with torch.no_grad():
                q_values = self.policy_net(torch.FloatTensor(state).unsqueeze(0))
                action_idx = q_values.argmax(dim=1).item()
            exploring = False

        q_vals = []
        if not exploring:
            q_vals = q_values.squeeze().tolist()

        return {
            "action": self.ACTIONS[action_idx],
            "action_idx": action_idx,
            "q_values": q_vals,
            "exploring": exploring,
        }

    def train_step(self) -> Optional[float]:
        """performs one training step using experience replay"""
        if self.policy_net is None or len(self.replay_buffer) < self.batch_size:
            return None

        states, actions, rewards, next_states, dones = self.replay_buffer.sample(self.batch_size)

        states_t = torch.FloatTensor(states)
        actions_t = torch.LongTensor(actions)
        rewards_t = torch.FloatTensor(rewards)
        next_states_t = torch.FloatTensor(next_states)
        dones_t = torch.FloatTensor(dones)

        # current Q values
        current_q = self.policy_net(states_t).gather(1, actions_t.unsqueeze(1)).squeeze()

        # target Q values (double DQN)
        with torch.no_grad():
            next_actions = self.policy_net(next_states_t).argmax(dim=1)
            next_q = self.target_net(next_states_t).gather(1, next_actions.unsqueeze(1)).squeeze()
            target_q = rewards_t + self.gamma * next_q * (1 - dones_t)

        loss = nn.MSELoss()(current_q, target_q)

        self.optimizer.zero_grad()
        loss.backward()
        torch.nn.utils.clip_grad_norm_(self.policy_net.parameters(), 1.0)
        self.optimizer.step()

        # decay epsilon
        self.epsilon = max(self.epsilon_min, self.epsilon * self.epsilon_decay)

        return loss.item()

    def update_target(self):
        """copies policy network weights to target network"""
        if self.target_net is not None and self.policy_net is not None:
            self.target_net.load_state_dict(self.policy_net.state_dict())

    def train_from_backtest(self, candles: list[dict], initial_balance: float = 10000,
                            episodes: int = 100) -> dict:
        """trains the agent from historical candle data (backtest simulation)"""
        if self.policy_net is None:
            return {"success": False, "reason": "pytorch not available"}

        if len(candles) < 30:
            return {"success": False, "reason": "insufficient candle data"}

        episode_rewards = []
        episode_pnls = []

        for episode in range(episodes):
            total_reward, final_pnl = self._run_episode(candles, initial_balance)
            episode_rewards.append(total_reward)
            episode_pnls.append(final_pnl)

            # update target network periodically
            if (episode + 1) % 10 == 0:
                self.update_target()

        # compute stats
        avg_reward = float(np.mean(episode_rewards[-20:]))
        avg_pnl = float(np.mean(episode_pnls[-20:]))
        best_pnl = float(max(episode_pnls))

        result = {
            "success": True,
            "episodes": episodes,
            "avg_reward_last_20": round(avg_reward, 4),
            "avg_pnl_last_20": round(avg_pnl, 2),
            "best_pnl": round(best_pnl, 2),
            "final_epsilon": round(self.epsilon, 4),
            "buffer_size": len(self.replay_buffer),
        }

        self.training_history.append(result)
        return result

    def _run_episode(self, candles: list[dict], initial_balance: float) -> tuple:
        """runs one training episode through the candle data"""
        balance = initial_balance
        position = 0.0  # units held
        entry_price = 0.0
        total_reward = 0.0

        for i in range(20, len(candles) - 1):
            state = self._extract_state(candles, i, balance, position, entry_price, initial_balance)
            action_info = self.get_action(state, explore=True)
            action_idx = action_info["action_idx"]

            # execute action
            current_price = candles[i]["close"]
            next_price = candles[i + 1]["close"]

            balance, position, entry_price = self._execute_action(
                action_idx, balance, position, entry_price, current_price,
            )

            # compute reward from next price change
            reward = self._compute_reward(
                position, current_price, next_price, balance, initial_balance,
            )
            total_reward += reward

            next_state = self._extract_state(
                candles, i + 1, balance, position, entry_price, initial_balance,
            )
            done = (i == len(candles) - 2) or (balance + position * current_price < initial_balance * 0.5)

            self.replay_buffer.push(state, action_idx, reward, next_state, done)

            # train
            self.train_step()

            if done:
                break

        # final PnL
        final_value = balance + position * candles[min(i + 1, len(candles) - 1)]["close"]
        pnl = final_value - initial_balance

        return total_reward, pnl

    def _extract_state(self, candles: list[dict], idx: int, balance: float,
                       position: float, entry_price: float,
                       initial_balance: float) -> np.ndarray:
        """extracts state features from candle data and portfolio state"""
        closes = [c["close"] for c in candles[max(0, idx - 19):idx + 1]]
        volumes = [c["volume"] for c in candles[max(0, idx - 19):idx + 1]]
        highs = [c["high"] for c in candles[max(0, idx - 19):idx + 1]]
        lows = [c["low"] for c in candles[max(0, idx - 19):idx + 1]]

        closes = np.array(closes)
        volumes = np.array(volumes)

        current_price = closes[-1]

        # price returns (last 5)
        returns = np.zeros(5)
        if len(closes) >= 6:
            r = np.diff(closes[-6:]) / closes[-6:-1] * 100
            returns[:len(r)] = r

        # position ratio
        portfolio_value = balance + position * current_price
        position_ratio = (position * current_price) / max(portfolio_value, 1)

        # unrealized PnL
        unrealized_pnl = 0.0
        if position > 0 and entry_price > 0:
            unrealized_pnl = (current_price - entry_price) / entry_price * 100

        # volatility (ATR-like)
        if len(highs) >= 5:
            h = np.array(highs[-5:])
            l = np.array(lows[-5:])
            atr = np.mean(h - l) / current_price * 100
        else:
            atr = 0.0

        # momentum (SMA ratio)
        sma = np.mean(closes[-10:]) if len(closes) >= 10 else current_price
        momentum = (current_price / sma - 1) * 100

        # volume ratio
        vol_avg = np.mean(volumes[-10:]) if len(volumes) >= 10 else 1
        vol_ratio = volumes[-1] / max(vol_avg, 1) if len(volumes) > 0 else 1.0

        # RSI-like
        if len(closes) >= 14:
            deltas = np.diff(closes[-15:])
            gains = np.mean(np.maximum(deltas, 0))
            losses = np.mean(np.maximum(-deltas, 0))
            rs = gains / max(losses, 1e-8)
            rsi = 100 - (100 / (1 + rs))
        else:
            rsi = 50.0

        state = np.array([
            *returns,           # 5 features: recent returns
            position_ratio,     # 1: position exposure
            unrealized_pnl,     # 1: unrealized profit/loss %
            atr,                # 1: volatility
            momentum,           # 1: momentum
            vol_ratio,          # 1: volume anomaly
            rsi / 100.0,        # 1: RSI normalized
            portfolio_value / initial_balance,  # 1: portfolio performance
        ], dtype=np.float32)

        return state

    def _execute_action(self, action_idx: int, balance: float, position: float,
                        entry_price: float, price: float) -> tuple:
        """executes a trading action and returns updated state"""
        if action_idx == 1:  # buy small
            amount = balance * 0.25
            if amount > 1:
                units = amount / price
                position += units
                balance -= amount
                if entry_price == 0:
                    entry_price = price
                else:
                    entry_price = (entry_price + price) / 2
        elif action_idx == 2:  # buy large
            amount = balance * 0.5
            if amount > 1:
                units = amount / price
                position += units
                balance -= amount
                if entry_price == 0:
                    entry_price = price
                else:
                    entry_price = (entry_price + price) / 2
        elif action_idx == 3:  # sell small
            sell_units = position * 0.25
            if sell_units > 0:
                balance += sell_units * price
                position -= sell_units
                if position < 1e-8:
                    position = 0
                    entry_price = 0
        elif action_idx == 4:  # sell all
            if position > 0:
                balance += position * price
                position = 0
                entry_price = 0

        return balance, position, entry_price

    def _compute_reward(self, position: float, current_price: float,
                        next_price: float, balance: float,
                        initial_balance: float) -> float:
        """computes reward based on portfolio change"""
        if position > 0:
            price_change = (next_price - current_price) / current_price
            reward = position * current_price * price_change / initial_balance * 100
        else:
            # small negative reward for holding cash (opportunity cost)
            reward = -0.001

        # penalty for large drawdowns
        portfolio = balance + position * current_price
        if portfolio < initial_balance * 0.9:
            reward -= 0.5

        return reward

    def save(self, path: str = "models/rl_agent.pt"):
        """saves the agent's policy network"""
        if self.policy_net is None:
            return
        os.makedirs(os.path.dirname(path) if os.path.dirname(path) else ".", exist_ok=True)
        torch.save({
            "policy_state": self.policy_net.state_dict(),
            "target_state": self.target_net.state_dict(),
            "epsilon": self.epsilon,
        }, path)

    def load(self, path: str = "models/rl_agent.pt"):
        """loads a saved agent"""
        if self.policy_net is None or not os.path.exists(path):
            return False
        checkpoint = torch.load(path, map_location="cpu")
        self.policy_net.load_state_dict(checkpoint["policy_state"])
        self.target_net.load_state_dict(checkpoint["target_state"])
        self.epsilon = checkpoint.get("epsilon", self.epsilon_min)
        return True


import os  # noqa: E402 — needed for save/load
