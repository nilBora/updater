package task

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/google/uuid"

	store "github.com/nilBora/updater/app/store"
)

func getStore() store.Store {
    sec := store.Store {
        StorePath: "/var/tmp/jtrw_updater_test.db",
    }

    sec.JBolt = sec.NewStore()

    return sec
}

func TestShellRunner_Run(t *testing.T) {
    sec := getStore()
	sr := ShellRunner{DataStore: sec}
	uuidStr := uuid.New().String()
	{
		lw := bytes.NewBuffer(nil)
		err := sr.Run(context.Background(), "echo 123", lw, uuidStr)
		t.Log(lw.String())
		require.NoError(t, err)
		assert.Equal(t, "123\n", lw.String())
	}

	{
	    uuidStr = uuid.New().String()
		lw := bytes.NewBuffer(nil)
		err := sr.Run(context.Background(), "no-such-command 123", lw, uuidStr)
		require.Error(t, err)
		t.Log(lw.String())
		assert.Contains(t, lw.String(), "not found")
	}

	{
		lw := bytes.NewBuffer(nil)
		err := sr.Run(context.Background(), "@no-such-command 123", lw, uuidStr)
		t.Log(lw.String())
		require.NoError(t, err)
		assert.Contains(t, lw.String(), "not found")
	}
}

func TestShellRunner_RunMultiLine(t *testing.T) {
    sec := getStore()
	sr := ShellRunner{DataStore: sec}
    uuidStr := uuid.New().String()
	{
		lw := bytes.NewBuffer(nil)
		err := sr.Run(context.Background(), "echo 123\necho 567\n", lw, uuidStr)
		require.NoError(t, err)
		assert.Equal(t, "123\n567\n", lw.String())
	}

	{
		lw := bytes.NewBuffer(nil)
		err := sr.Run(context.Background(), "echo 123\nno-such-command 123", lw, uuidStr)
		require.Error(t, err)
		assert.Contains(t, lw.String(), "not found")
	}

	{
		lw := bytes.NewBuffer(nil)
		err := sr.Run(context.Background(), "echo 123\n@no-such-command 123", lw, uuidStr)
		require.NoError(t, err)
		assert.Contains(t, lw.String(), "not found")
	}

}
func TestShellRunner_RunBatch(t *testing.T) {
    sec := getStore()
	sr := ShellRunner{BatchMode: true, TimeOut: time.Second, DataStore: sec}
	lw := bytes.NewBuffer(nil)
	uuidStr := uuid.New().String()
	err := sr.Run(context.Background(), "echo 123\necho 345", lw, uuidStr)
	require.NoError(t, err)
	assert.Equal(t, "123\n345\n", lw.String())
}

func TestShellRunner_RunBatchTimeOut(t *testing.T) {
    sec := getStore()
	sr := ShellRunner{BatchMode: true, TimeOut: time.Millisecond * 100, DataStore: sec}
	lw := bytes.NewBuffer(nil)
	st := time.Now()
	uuidStr := uuid.New().String()

	err := sr.Run(context.Background(), "sleep 1 && sleep 1 && echo 123\necho 345", lw, uuidStr)
	require.Error(t, err)
	assert.True(t, time.Since(st) < time.Second*2)
}
