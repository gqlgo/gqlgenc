package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"

	"github.com/Yamashou/gqlgenc/v3/config"
)

// outputBackupSuffix は、生成の間だけ退避する前回の出力ファイルの接尾辞。.go で終わらないため、
// 生成中のパッケージのロードは退避したファイルを読まない (出力を消したのと同じに見える)。
const outputBackupSuffix = ".gqlgenc.bak"

// outputBackupState は、1 つの出力ファイルの生成中の状態を表す。
//
//	backupOutput ──▶ PreviousSaved ──commit──▶ Committed
//	            └──▶ NoPrevious    ──restore─▶ Restored
//
// PreviousSaved と NoPrevious は未確定の状態で、それぞれ 1 回だけ Committed か Restored へ進む。
// commit と restore は、失敗したときに状態を変えない。そのため失敗の後でも restore をやり直せる。
type outputBackupState int

const (
	// outputBackupStatePreviousSaved は、前回の出力を bakPath に退避してある状態。
	outputBackupStatePreviousSaved outputBackupState = iota + 1
	// outputBackupStateNoPrevious は、退避する前回の出力が無かった状態。
	outputBackupStateNoPrevious
	// outputBackupStateCommitted は、生成の結果を採用した状態。
	outputBackupStateCommitted
	// outputBackupStateRestored は、前回の出力を元へ戻した状態。
	outputBackupStateRestored
)

// isSettled は、commit か restore を終えた状態かを返す。
func (s outputBackupState) isSettled() bool {
	return s == outputBackupStateCommitted || s == outputBackupStateRestored
}

// outputBackup は、生成の間だけ退避した 1 つの出力ファイルを表す。
type outputBackup struct {
	path    string
	bakPath string
	state   outputBackupState
}

// backupOutputs は、設定された出力ファイル (model・query・client) をそれぞれ退避する。
//
// 生成の前に出力を出力先から外すのは、出力先のパッケージを束縛先としてロードするときに、
// 前回の生成物の型を手書きの型として拾わないためである。消さずに退避することで、
// 生成が失敗したときに前回の出力を戻せる。
//
// 事前条件: 作業ディレクトリが、出力ファイルの相対パスの基準になっている。
// 事後条件: 成功したら、設定された出力ファイルは出力先に無い。失敗したら、それまでに退避した
// 出力を元へ戻してからエラーを返す。
func backupOutputs(cfg *config.Config) ([]*outputBackup, error) {
	outputs := []*gqlgenconfig.PackageConfig{
		&cfg.GQLGenConfig.Model,
		&cfg.GQLGencConfig.QueryGen,
		&cfg.GQLGencConfig.ClientGen,
	}
	backups := make([]*outputBackup, 0, len(outputs))

	for _, output := range outputs {
		if !output.IsDefined() {
			continue
		}

		evictOutputPackage(cfg, output)

		backup, err := backupOutput(output.Filename)
		if err != nil {
			if restoreErr := restoreOutputBackups(backups); restoreErr != nil {
				return nil, fmt.Errorf("%w; failed to restore outputs: %w", err, restoreErr)
			}

			return nil, err
		}

		backups = append(backups, backup)
	}

	return backups, nil
}

// evictOutputPackage は、出力先のパッケージをキャッシュから外す。複数の設定を 1 プロセスで
// 処理するとき、前の設定がこのパッケージを退避前のファイル込みでロードしていることがある。
// そのキャッシュを使い続けると、退避した前回の生成物の型を束縛先の型として拾ってしまう。
func evictOutputPackage(cfg *config.Config, output *gqlgenconfig.PackageConfig) {
	if cfg.GQLGenConfig.Packages != nil {
		cfg.GQLGenConfig.Packages.Evict(output.ImportPath())
	}

	if cfg.GQLGencConfig.TypeBinder != nil {
		if dir, err := filepath.Abs(output.Dir()); err == nil {
			cfg.GQLGencConfig.TypeBinder.EvictDir(dir)
		}
	}
}

// backupOutput は、path のファイルを path + outputBackupSuffix へ退避する。
//
// 退避したファイルは commit か restore でしか消えない。開始時に残っている退避ファイルは、
// どちらも終えずに中断した実行が残したもので、最後に成功した出力を保持している。そのとき
// 出力先にあるファイルは中断した実行の書きかけなので、退避ファイルをそれで上書きせずに消し、
// 残っていた退避ファイルを前回の出力として扱う。
//
// 事後条件: 成功したら path にファイルは無い。前回の出力があれば bakPath にだけある。
func backupOutput(path string) (*outputBackup, error) {
	backup := &outputBackup{
		path:    path,
		bakPath: path + outputBackupSuffix,
		state:   outputBackupStateNoPrevious,
	}

	_, err := os.Stat(backup.bakPath)
	if err == nil {
		if err := os.Remove(backup.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("remove output of an interrupted run: %w", err)
		}

		backup.state = outputBackupStatePreviousSaved

		return backup, nil
	}

	if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("stat output backup: %w", err)
	}

	if err := os.Rename(backup.path, backup.bakPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// 前回の出力が無い (初回の生成)
			return backup, nil
		}

		return nil, fmt.Errorf("back up output: %w", err)
	}

	backup.state = outputBackupStatePreviousSaved

	return backup, nil
}

// restoreOutputBackups は、未確定の退避をすべて元へ戻す (退避した順の逆順)。確定済みの退避には
// 何もしないため、commit の後や 2 回目の呼び出しは何も変えない。
//
// 事後条件: 成功したら、未確定だった退避のうち前回の出力があったものは出力先に戻っていて、
// 退避ファイルは残っていない。
func restoreOutputBackups(backups []*outputBackup) error {
	errs := make([]error, 0, len(backups))

	for _, backup := range slices.Backward(backups) {
		if err := backup.restore(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// restore は前回の出力を元へ戻し、未確定の退避を Restored にする。確定済みの退避には何もしない。
// 失敗したときは状態を変えない。
func (b *outputBackup) restore() error {
	switch b.state {
	case outputBackupStateCommitted, outputBackupStateRestored:
		return nil
	case outputBackupStateNoPrevious:
		// 前回の出力が無かったので、生成が書いた (書きかけの) ファイルを消す
		if err := os.Remove(b.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove partial output: %w", err)
		}
	case outputBackupStatePreviousSaved:
		// Rename は書きかけの出力を置き換える。退避ファイルが既に無ければ、出力先には触らない
		if err := os.Rename(b.bakPath, b.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("restore output: %w", err)
		}
	default:
		panic(fmt.Sprintf("output backup of %s: unknown state %d", b.path, b.state))
	}

	b.state = outputBackupStateRestored

	return nil
}

// outputCommitAction は、1 つの出力の commit がファイルに対して行うこと。どのファイルにも
// 触る前に、すべての出力について決める。
type outputCommitAction int

const (
	// outputCommitActionNone は、片付ける退避ファイルが無い (前回の出力が無かったか、確定済み)。
	outputCommitActionNone outputCommitAction = iota + 1
	// outputCommitActionDropBackup は、生成が出力先にファイルを残さなかったので、退避ファイルを消す。
	outputCommitActionDropBackup
	// outputCommitActionKeepPrevious は、生成した内容が退避ファイルと同じなので、退避ファイルを
	// 出力先へ戻す (前回の更新時刻を保つ)。
	outputCommitActionKeepPrevious
	// outputCommitActionKeepGenerated は、生成した内容が退避ファイルと違うので、退避ファイルを消す。
	outputCommitActionKeepGenerated
)

// commitOutputBackups は生成の結果を採用する。生成した内容が前回と同じ出力は前回のファイルを
// 戻して更新時刻を保ち、違う出力は生成したファイルを残す。
//
// 一部の出力だけが採用されて残りが戻る状態を避けるため、2 段階で行う。まずすべての出力を
// 確かめて読み、全部が通ってから退避ファイルを消すか戻す。
//
// 事前条件: 生成 (removePackageClauseOnlyModel を含む) が終わっている。
// 事後条件: 成功したら、すべての退避が確定していて退避ファイルは残っていない。出力を確かめる
// 段階で失敗したら、どの退避も確定しておらず、どのファイルも変えていない。
func commitOutputBackups(backups []*outputBackup) error {
	actions := make([]outputCommitAction, 0, len(backups))

	for _, backup := range backups {
		action, err := backup.planCommit()
		if err != nil {
			return err
		}

		actions = append(actions, action)
	}

	for i, backup := range backups {
		if err := backup.applyCommit(actions[i]); err != nil {
			return err
		}
	}

	return nil
}

// planCommit は、commit がこの出力に対して行うことを、ファイルを変えずに決める。
func (b *outputBackup) planCommit() (outputCommitAction, error) {
	if b.state != outputBackupStatePreviousSaved {
		return outputCommitActionNone, nil
	}

	generated, err := os.ReadFile(b.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// 宣言の無い model ファイルを消した場合など、生成が出力先に何も残さなかった
			return outputCommitActionDropBackup, nil
		}

		return 0, fmt.Errorf("read generated output: %w", err)
	}

	previous, err := os.ReadFile(b.bakPath)
	if err != nil {
		return 0, fmt.Errorf("read output backup: %w", err)
	}

	if bytes.Equal(generated, previous) {
		return outputCommitActionKeepPrevious, nil
	}

	return outputCommitActionKeepGenerated, nil
}

// applyCommit は planCommit が決めたことを行い、未確定の退避を Committed にする。確定済みの
// 退避には何もしない。失敗したときは状態を変えないため、restore で前回の出力を戻せる。
//
// 事前条件: action は b.planCommit がエラー無しで返した値で、その後ファイルは変わっていない。
func (b *outputBackup) applyCommit(action outputCommitAction) error {
	if b.state.isSettled() {
		return nil
	}

	switch action {
	case outputCommitActionNone:
	case outputCommitActionDropBackup, outputCommitActionKeepGenerated:
		if err := os.Remove(b.bakPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove output backup: %w", err)
		}
	case outputCommitActionKeepPrevious:
		// Rename は生成したファイルを置き換え、前回の更新時刻を保つ
		if err := os.Rename(b.bakPath, b.path); err != nil {
			return fmt.Errorf("restore unchanged output: %w", err)
		}
	default:
		panic(fmt.Sprintf("output backup of %s: unknown commit action %d", b.path, action))
	}

	b.state = outputBackupStateCommitted

	return nil
}
