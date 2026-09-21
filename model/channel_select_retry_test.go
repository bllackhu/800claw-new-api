package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func withChannelsIDM(t *testing.T, channels []*Channel) {
	t.Helper()
	original := channelsIDM
	t.Cleanup(func() { channelsIDM = original })

	m := make(map[int]*Channel, len(channels))
	for _, c := range channels {
		m[c.Id] = c
	}
	channelsIDM = m
}

func newTestChannel(id int, priority int64, weight uint) *Channel {
	p := priority
	w := weight
	return &Channel{Id: id, Priority: &p, Weight: &w}
}

func TestGetRandomChannelFromIDsTierExhaustion(t *testing.T) {
	// priority 20: {8, 11, 12}; priority 19: {10, 4}
	all := []*Channel{
		newTestChannel(8, 20, 100),
		newTestChannel(11, 20, 100),
		newTestChannel(12, 20, 100),
		newTestChannel(10, 19, 100),
		newTestChannel(4, 19, 100),
	}
	withChannelsIDM(t, all)
	ids := []int{8, 11, 12, 10, 4}

	t.Run("retries exhaust top tier before dropping priority", func(t *testing.T) {
		used := map[int]bool{}
		seen := map[int]bool{}
		for i := 0; i < 3; i++ {
			ch, err := getRandomChannelFromIDs(ids, i, used)
			require.NoError(t, err)
			require.NotNil(t, ch)
			require.Equal(t, int64(20), ch.GetPriority())
			require.False(t, seen[ch.Id], "channel %d selected twice within tier", ch.Id)
			seen[ch.Id] = true
			used[ch.Id] = true
		}
		require.Len(t, seen, 3, "all three priority-20 channels should be tried first")

		// tier 20 exhausted -> drop to 19
		ch, err := getRandomChannelFromIDs(ids, 3, used)
		require.NoError(t, err)
		require.NotNil(t, ch)
		require.Equal(t, int64(19), ch.GetPriority())
	})

	t.Run("returns nil when all channels tried", func(t *testing.T) {
		used := map[int]bool{8: true, 11: true, 12: true, 10: true, 4: true}
		ch, err := getRandomChannelFromIDs(ids, 5, used)
		require.NoError(t, err)
		require.Nil(t, ch)
	})

	t.Run("single channel already used returns nil", func(t *testing.T) {
		ch, err := getRandomChannelFromIDs([]int{8}, 1, map[int]bool{8: true})
		require.NoError(t, err)
		require.Nil(t, ch)
	})
}

func TestPickHighestUntriedTier(t *testing.T) {
	p20 := int64(20)
	p19 := int64(19)
	abilities := []Ability{
		{ChannelId: 8, Priority: &p20},
		{ChannelId: 11, Priority: &p20},
		{ChannelId: 12, Priority: &p20},
		{ChannelId: 10, Priority: &p19},
	}

	t.Run("picks highest tier with untried channel", func(t *testing.T) {
		candidates := pickHighestUntriedTier(abilities, map[int]bool{})
		require.Len(t, candidates, 3)
		for _, a := range candidates {
			require.Equal(t, int64(20), *a.Priority)
		}
	})

	t.Run("excludes used channels within tier", func(t *testing.T) {
		candidates := pickHighestUntriedTier(abilities, map[int]bool{8: true})
		require.Len(t, candidates, 2)
		for _, a := range candidates {
			require.NotEqual(t, 8, a.ChannelId)
		}
	})

	t.Run("drops to lower tier once top exhausted", func(t *testing.T) {
		candidates := pickHighestUntriedTier(abilities, map[int]bool{8: true, 11: true, 12: true})
		require.Len(t, candidates, 1)
		require.Equal(t, 10, candidates[0].ChannelId)
	})

	t.Run("returns nil when all used", func(t *testing.T) {
		candidates := pickHighestUntriedTier(abilities, map[int]bool{8: true, 11: true, 12: true, 10: true})
		require.Nil(t, candidates)
	})
}
