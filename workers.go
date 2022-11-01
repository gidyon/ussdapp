package ussdapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	tickerInterval = 5 * time.Second
	failedBulkDir  = "failed-bulk-inserts"
	bulkInsertSize = 1000
)

func (app *UssdApp) saveLogsWorker(ctx context.Context) {
	if !app.opt.SaveLogs {
		return
	}

	ticker := time.NewTicker(tickerInterval)
	defer ticker.Stop()

	if !app.opt.SQLDB.Migrator().HasTable(&SessionRequest{}) {
		err := app.opt.SQLDB.Migrator().AutoMigrate(&SessionRequest{})
		if err != nil {
			app.Logger().Fatal(err)
		}
	}

	var (
		currCap = bulkInsertSize
		logs    = make([]*SessionRequest, 0, bulkInsertSize)
		err     error

		updateCap = func() {
			// Check that channel if filled
			if len(app.logsChan) == cap(app.logsChan) {
				currCap = currCap + (currCap / 2)
				app.opt.Logger.Infof("INSERT USSD LOGS: channel is filled, draining and expanding channel to capacity %d", currCap)

				// Drain the channel
				for v := range app.logsChan {
					logs = append(logs, v)
				}

				// Update channel capacity
				app.logsChan = make(chan *SessionRequest, currCap)
			}
		}

		callback = func() error {
			// We update logs and capacity regardless
			defer func() {
				logs = logs[0:0]
				updateCap()
			}()

			return app.opt.SQLDB.Transaction(func(tx *gorm.DB) error {
				err1 := tx.CreateInBatches(logs, len(logs)+1).Error
				if err1 != nil {
					tx.Rollback()
					app.opt.Logger.Errorf("INSERT USSD LOGS FAILED (SAVING LOGS IN FILE ...): %v", err1)

					// Get directory
					_, err := os.Stat(failedBulkDir)
					switch {
					case err == nil:
					case os.IsNotExist(err):
						err := os.Mkdir(failedBulkDir, 0755)
						if err != nil {
							return err
						}
					default:
						return err
					}

					fileName := fmt.Sprintf("%s/bulk-%d.json", failedBulkDir, time.Now().UnixNano())

					// Save logs locally in file
					f, err := os.Create(fileName)
					if err != nil {
						app.opt.Logger.Errorf("FAILED TO CREATE FILE: %v", err)
						return err
					}
					defer f.Close()

					err = json.NewEncoder(f).Encode(logs)
					if err != nil {
						app.opt.Logger.Errorf("FAILED TO ADD JSON DATA TO FILE: %v", err)
						return err
					}

					return err1
				}

				return nil
			})
		}
	)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logsLen := len(logs)
			if logsLen > 0 {
				err = callback()
				if err == nil {
					app.opt.Logger.Infof("INSERT USSD LOGS: bulk inserted %d ussd logs from ticker", logsLen)
					ticker.Reset(tickerInterval)
				} else {
					app.opt.Logger.Errorf("INSERT USSD LOGS: failed to bulk insert: %v", err)
				}
			}

		case logDB := <-app.logsChan:
			logs = append(logs, logDB)
			logsLen := len(logs)
			if logsLen > currCap-1 {
				err = callback()
				if err == nil {
					app.opt.Logger.Infof("INSERT USSD LOGS: bulk inserted %d ussd logs from channel", logsLen)
					ticker.Reset(tickerInterval)
				} else {
					app.opt.Logger.Errorf("INSERT USSD LOGS: failed to bulk insert: %v", err)
				}
			}
		}
	}

}

func (app *UssdApp) saveFailedLogsWorker(ctx context.Context) {
	timer := time.NewTicker(30 * time.Second)
	defer timer.Stop()

	_, err := os.Stat(failedBulkDir)
	switch {
	case err == nil:
	case os.IsNotExist(err):
		err := os.Mkdir(failedBulkDir, 0755)
		if err != nil {
			app.opt.Logger.Errorf("failed to create directory: %v", err)
		}
	default:
		app.opt.Logger.Errorf("failed to create directory: %v", err)
	}

loop:
	for range timer.C {
		// Read from directories and try to save logs that have failed
		filesInfo, err := ioutil.ReadDir(failedBulkDir)
		if err != nil {
			app.opt.Logger.Warningf("SAVE FAILED LOGS WORKER: failed to read directory: %v", err)
			continue
		}

	loopIn:
		for _, fileInfo := range filesInfo {
			fileName := filepath.Join(failedBulkDir, fileInfo.Name())

			// Read file content
			buf, err := ioutil.ReadFile(fileName)
			if err != nil {
				app.opt.Logger.Warningf("SAVE FAILED LOGS WORKER: failed to read file contents: %v", err)
				continue
			}

			logs := make([]*SessionRequest, 0, bulkInsertSize)

			err = json.Unmarshal(buf, &logs)
			if err != nil {
				app.opt.Logger.Warningf("SAVE FAILED LOGS WORKER: failed to unmarshal file contents: %v", err)
				continue
			}

			const safeBulkSize = 1000

			tx := app.opt.SQLDB.Begin()
			if tx.Error != nil {
				app.opt.Logger.Warningf("SAVE FAILED LOGS WORKER: failed to begin transaction: %v", err)
				continue
			}

			tx = tx.Clauses(clause.OnConflict{DoNothing: true})

			if len(logs) > safeBulkSize {
				for i, j := 0, safeBulkSize; i < len(logs); i, j = i+safeBulkSize, j+safeBulkSize {
					to := j
					if j > len(logs) {
						to = len(logs)
					}
					logsIn := logs[i:to]

					// Create batches of the logs
					err = tx.CreateInBatches(logsIn, len(logs)+1).Error
					if err != nil {
						tx.Rollback()
						app.opt.Logger.Warningf("SAVE FAILED LOGS WORKER: failed to save file batch: %v", err)
						goto loopIn
					}
				}
			} else {
				err = tx.CreateInBatches(logs, len(logs)+1).Error
				if err != nil {
					tx.Rollback()
					app.opt.Logger.Warningf("SAVE FAILED LOGS WORKER: failed to save file batch: %v", err)
					goto loop
				}
			}

			// Commit transaction
			err = tx.Commit().Error
			if err != nil {
				tx.Rollback()
				app.opt.Logger.Warningf("SAVE FAILED LOGS WORKER: failed to commit transaction: %v", err)
				continue
			}

			logs = logs[0:0]

			// Delete the file
			err = os.Remove(fileName)
			if err != nil {
				app.opt.Logger.Warningf("SAVE FAILED LOGS WORKER: failed to remove file: %v", err)
				goto loop
			}

			app.opt.Logger.Warningf("SAVE FAILED LOGS WORKER: successfully saved contents of file: %s", fileName)
		}
	}
}
