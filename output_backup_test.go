package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// previousModTime は、テストが退避ファイルに付ける更新時刻。commit が前回のファイルを戻したか
// (生成したファイルで置き換えていないか) を、更新時刻で見分けるために使う。
var previousModTime = time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)

// writeOutputFiles は dir に files を書く。名前が "/" で終わるものはディレクトリを作る。
// 退避ファイル (.gqlgenc.bak) には previousModTime を付ける。
func writeOutputFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for name, content := range files {
		path := filepath.Join(dir, name)
		if strings.HasSuffix(name, "/") {
			if err := os.Mkdir(path, 0o750); err != nil {
				t.Fatalf("failed to create directory %s: %v", name, err)
			}
			continue
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
		if strings.HasSuffix(name, outputBackupSuffix) {
			if err := os.Chtimes(path, previousModTime, previousModTime); err != nil {
				t.Fatalf("failed to set mod time of %s: %v", name, err)
			}
		}
	}
}

// readOutputFiles は dir のファイルの名前と内容を返す。ディレクトリは名前に "/" を付け、内容を空にする。
func readOutputFiles(t *testing.T, dir string) map[string]string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}

	files := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			files[entry.Name()+"/"] = ""
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("failed to read %s: %v", entry.Name(), err)
		}
		files[entry.Name()] = string(content)
	}

	return files
}

// previousKeptFiles は、dir のファイルのうち更新時刻が previousModTime のものの名前を名前順で返す。
func previousKeptFiles(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}

	var names []string
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatalf("failed to stat %s: %v", entry.Name(), err)
		}
		if info.ModTime().Equal(previousModTime) {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)

	return names
}

func TestBackupOutput(t *testing.T) {
	t.Parallel()

	type args struct {
		// files は、実行前に出力先のディレクトリにあるファイル
		files map[string]string
	}

	type want struct {
		backup *outputBackup
		files  map[string]string
		err    error
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// 前回の出力は出力先から外し、退避ファイルにだけ残す
			name: "前回の出力があれば退避ファイルへ移す",
			args: args{
				files: map[string]string{"query_gen.go": "previous"},
			},
			want: want{
				backup: &outputBackup{state: outputBackupStatePreviousSaved},
				files:  map[string]string{"query_gen.go.gqlgenc.bak": "previous"},
			},
		},
		{
			// 初回の生成では退避するものが無く、エラーにもならない
			name: "前回の出力が無ければ何も退避しない",
			args: args{
				files: map[string]string{},
			},
			want: want{
				backup: &outputBackup{state: outputBackupStateNoPrevious},
				files:  map[string]string{},
			},
		},
		{
			// 残っている退避ファイルは最後に成功した出力で、出力先にあるのは中断した実行の書きかけ。
			// 書きかけで退避ファイルを上書きすると、最後に成功した出力を失う (gqlgo/gqlgenc#375)
			name: "中断した実行の退避ファイルが残っていれば、それを前回の出力として残し書きかけを消す",
			args: args{
				files: map[string]string{
					"query_gen.go":             "partial",
					"query_gen.go.gqlgenc.bak": "last good",
				},
			},
			want: want{
				backup: &outputBackup{state: outputBackupStatePreviousSaved},
				files:  map[string]string{"query_gen.go.gqlgenc.bak": "last good"},
			},
		},
		{
			// 退避ファイルだけが残っている (中断した実行が出力を書く前に終わった) 場合も前回の出力として扱う
			name: "退避ファイルだけが残っていれば、それを前回の出力として残す",
			args: args{
				files: map[string]string{"query_gen.go.gqlgenc.bak": "last good"},
			},
			want: want{
				backup: &outputBackup{state: outputBackupStatePreviousSaved},
				files:  map[string]string{"query_gen.go.gqlgenc.bak": "last good"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeOutputFiles(t, dir, tt.args.files)

			got, err := backupOutput(filepath.Join(dir, "query_gen.go"))

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}

			// path と bakPath は一時ディレクトリの下でテストごとに変わるため、比較から外す
			if diff := cmp.Diff(tt.want.backup, got, cmp.AllowUnexported(outputBackup{}), cmpopts.IgnoreFields(outputBackup{}, "path", "bakPath")); diff != "" {
				t.Errorf("backup diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(tt.want.files, readOutputFiles(t, dir)); diff != "" {
				t.Errorf("files diff(-want +got): %s", diff)
			}
		})
	}
}

func TestOutputBackup_restore(t *testing.T) {
	t.Parallel()

	type fields struct {
		state outputBackupState
	}

	type args struct {
		// files は、実行前に出力先のディレクトリにあるファイル
		files map[string]string
	}

	type want struct {
		state outputBackupState
		files map[string]string
		err   error
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			// 生成が途中まで書いたファイルを、前回の出力で置き換える
			name: "前回の出力を退避してあれば、書きかけの出力を前回の出力で置き換える",
			fields: fields{
				state: outputBackupStatePreviousSaved,
			},
			args: args{
				files: map[string]string{
					"query_gen.go":             "partial",
					"query_gen.go.gqlgenc.bak": "previous",
				},
			},
			want: want{
				state: outputBackupStateRestored,
				files: map[string]string{"query_gen.go": "previous"},
			},
		},
		{
			// 生成が出力を書く前に失敗した場合も、前回の出力を戻す
			name: "前回の出力を退避してあり出力が書かれていなければ、前回の出力を戻す",
			fields: fields{
				state: outputBackupStatePreviousSaved,
			},
			args: args{
				files: map[string]string{"query_gen.go.gqlgenc.bak": "previous"},
			},
			want: want{
				state: outputBackupStateRestored,
				files: map[string]string{"query_gen.go": "previous"},
			},
		},
		{
			// 前回の出力が無かった出力先には、失敗した生成の書きかけを残さない
			name: "前回の出力が無ければ、書きかけの出力を消す",
			fields: fields{
				state: outputBackupStateNoPrevious,
			},
			args: args{
				files: map[string]string{"query_gen.go": "partial"},
			},
			want: want{
				state: outputBackupStateRestored,
				files: map[string]string{},
			},
		},
		{
			// 生成が成功して採用した出力を、後から呼ばれた restore が消してはいけない
			name: "採用済みの出力には何もしない",
			fields: fields{
				state: outputBackupStateCommitted,
			},
			args: args{
				files: map[string]string{"query_gen.go": "generated"},
			},
			want: want{
				state: outputBackupStateCommitted,
				files: map[string]string{"query_gen.go": "generated"},
			},
		},
		{
			// 戻し終えた出力に対する 2 回目の restore は、戻した出力を消さない
			name: "戻し終えた出力には何もしない",
			fields: fields{
				state: outputBackupStateRestored,
			},
			args: args{
				files: map[string]string{"query_gen.go": "previous"},
			},
			want: want{
				state: outputBackupStateRestored,
				files: map[string]string{"query_gen.go": "previous"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeOutputFiles(t, dir, tt.args.files)

			path := filepath.Join(dir, "query_gen.go")
			backup := &outputBackup{path: path, bakPath: path + outputBackupSuffix, state: tt.fields.state}

			err := backup.restore()

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(tt.want.state, backup.state); diff != "" {
				t.Errorf("state diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(tt.want.files, readOutputFiles(t, dir)); diff != "" {
				t.Errorf("files diff(-want +got): %s", diff)
			}
		})
	}
}

func TestCommitOutputBackups(t *testing.T) {
	t.Parallel()

	type args struct {
		// files は、生成が終わった時点で出力先のディレクトリにあるファイル
		files map[string]string
		// states は、query_gen.go・client_gen.go の順の退避の状態
		states []outputBackupState
	}

	type want struct {
		states []outputBackupState
		files  map[string]string
		// previousKept は、前回のファイル (更新時刻が前回のまま) が残っている名前
		previousKept []string
		err          error
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// 内容が変わらない出力を書き換えると、更新時刻だけが変わって下流のビルドが走り直す
			name: "生成した内容が前回と同じ出力は、前回のファイルを戻して更新時刻を保つ",
			args: args{
				files: map[string]string{
					"query_gen.go":              "same",
					"query_gen.go.gqlgenc.bak":  "same",
					"client_gen.go":             "same",
					"client_gen.go.gqlgenc.bak": "same",
				},
				states: []outputBackupState{outputBackupStatePreviousSaved, outputBackupStatePreviousSaved},
			},
			want: want{
				states:       []outputBackupState{outputBackupStateCommitted, outputBackupStateCommitted},
				files:        map[string]string{"query_gen.go": "same", "client_gen.go": "same"},
				previousKept: []string{"client_gen.go", "query_gen.go"},
			},
		},
		{
			// 内容が変わった出力だけを生成したファイルにし、変わらない出力は前回のファイルのままにする
			name: "生成した内容が前回と違う出力は、生成したファイルを残して退避ファイルを消す",
			args: args{
				files: map[string]string{
					"query_gen.go":              "generated",
					"query_gen.go.gqlgenc.bak":  "previous",
					"client_gen.go":             "same",
					"client_gen.go.gqlgenc.bak": "same",
				},
				states: []outputBackupState{outputBackupStatePreviousSaved, outputBackupStatePreviousSaved},
			},
			want: want{
				states:       []outputBackupState{outputBackupStateCommitted, outputBackupStateCommitted},
				files:        map[string]string{"query_gen.go": "generated", "client_gen.go": "same"},
				previousKept: []string{"client_gen.go"},
			},
		},
		{
			// 宣言の無い model ファイルを消した場合など、今回は出力が無いのに前回の出力を残してはいけない
			name: "生成が出力先にファイルを残さなければ、退避ファイルを消す",
			args: args{
				files: map[string]string{
					"query_gen.go.gqlgenc.bak": "previous",
					"client_gen.go":            "generated",
				},
				states: []outputBackupState{outputBackupStatePreviousSaved, outputBackupStateNoPrevious},
			},
			want: want{
				states: []outputBackupState{outputBackupStateCommitted, outputBackupStateCommitted},
				files:  map[string]string{"client_gen.go": "generated"},
			},
		},
		{
			// 1 つ目を採用してから 2 つ目で失敗すると、1 つ目の前回の出力を戻せなくなる (gqlgo/gqlgenc#376)。
			// 2 つ目の出力先をディレクトリにして、読めない出力を再現する
			name: "読めない出力があれば、どの出力も採用せずどのファイルも変えない",
			args: args{
				files: map[string]string{
					"query_gen.go":              "generated",
					"query_gen.go.gqlgenc.bak":  "previous",
					"client_gen.go/":            "",
					"client_gen.go.gqlgenc.bak": "previous",
				},
				states: []outputBackupState{outputBackupStatePreviousSaved, outputBackupStatePreviousSaved},
			},
			want: want{
				states: []outputBackupState{outputBackupStatePreviousSaved, outputBackupStatePreviousSaved},
				files: map[string]string{
					"query_gen.go":              "generated",
					"query_gen.go.gqlgenc.bak":  "previous",
					"client_gen.go/":            "",
					"client_gen.go.gqlgenc.bak": "previous",
				},
				previousKept: []string{"client_gen.go.gqlgenc.bak", "query_gen.go.gqlgenc.bak"},
				err:          cmpopts.AnyError,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeOutputFiles(t, dir, tt.args.files)

			backups := make([]*outputBackup, 0, len(tt.args.states))
			for i, name := range []string{"query_gen.go", "client_gen.go"} {
				path := filepath.Join(dir, name)
				backups = append(backups, &outputBackup{path: path, bakPath: path + outputBackupSuffix, state: tt.args.states[i]})
			}

			err := commitOutputBackups(backups)

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}

			gotStates := make([]outputBackupState, 0, len(backups))
			for _, backup := range backups {
				gotStates = append(gotStates, backup.state)
			}
			if diff := cmp.Diff(tt.want.states, gotStates); diff != "" {
				t.Errorf("states diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(tt.want.files, readOutputFiles(t, dir)); diff != "" {
				t.Errorf("files diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(tt.want.previousKept, previousKeptFiles(t, dir), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("previousKept diff(-want +got): %s", diff)
			}
		})
	}
}
