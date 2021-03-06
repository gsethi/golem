/*
   Copyright (C) 2003-2011 Institute for Systems Biology
                           Seattle, Washington, USA.

   This library is free software; you can redistribute it and/or
   modify it under the terms of the GNU Lesser General Public
   License as published by the Free Software Foundation; either
   version 2.1 of the License, or (at your option) any later version.

   This library is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
   Lesser General Public License for more details.

   You should have received a copy of the GNU Lesser General Public
   License along with this library; if not, write to the Free Software
   Foundation, Inc., 59 Temple Place, Suite 330, Boston, MA 02111-1307  USA

*/
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// TODO: Kill submissions once they finish.
type Submission struct {
	Details chan JobDetails
	Tasks   []Task

	CoutFileChan  chan string
	CerrFileChan  chan string
	ErrorChan     chan *WorkerJob
	FinishedChan  chan *WorkerJob
	SubmittedChan chan *SubmitedWorkerJob
	stopChan      chan int
	doneChan      chan int
}

func NewSubmission(jd JobDetails, tasks []Task, jobChan chan *WorkerJob) *Submission {
	logger.Debug("NewSubmission(%v)", jd)
	s := Submission{
		Details:       make(chan JobDetails, 1),
		Tasks:         tasks,
		CoutFileChan:  make(chan string, iobuffersize),
		CerrFileChan:  make(chan string, iobuffersize),
		ErrorChan:     make(chan *WorkerJob, 1),
		FinishedChan:  make(chan *WorkerJob, 1),
		SubmittedChan: make(chan *SubmitedWorkerJob, 1),
		stopChan:      make(chan int, 3),
		doneChan:      make(chan int, 0)}

	s.Details <- jd

	go s.MonitorWorkTasks()
	go s.WriteCout()
	go s.WriteCerror()
	go s.SubmitJobs(jobChan)

	return &s
}

// stops running job, returns true if job was still running
func (this *Submission) Stop() bool {
	logger.Debug("Stop()")
	dtls := this.SniffDetails()
	logger.Debug("Stop(): %v", dtls.JobId)

	if dtls.State == RUNNING {
		select {
		case this.stopChan <- 1:
			this.SetState(COMPLETE, STOPPED)
			logger.Debug("Stop():%v", this.SniffDetails())
		case <-time.After(250000000):
			logger.Printf("Stop(): timeout stopping: %v", dtls.JobId)
		}
		return true
	}
	return false
}

func (this *Submission) SniffDetails() JobDetails {
	dtls := <-this.Details
	this.Details <- dtls
	logger.Debug("SniffDetails(): dtls=%v", dtls)
	return dtls
}

func (this *Submission) MonitorWorkTasks() {
	logger.Debug("MonitorWorkTasks()")
	dtls := <-this.Details
	logFile, err := os.Create(fmt.Sprintf("%v.log.txt", dtls.JobId))
	if err != nil {
		logger.Warn(err)
	}
	this.Details <- dtls
	defer logFile.Close()

	for {
		select {
		case wj := <-this.ErrorChan:
			dtls := <-this.Details
			dtls.Progress.Errored = 1 + dtls.Progress.Errored
			dtls.LastModified = time.Now().String()
			this.Details <- dtls
			fmt.Fprintf(logFile, "ERRORED %v %v %v %v\n", wj.SubId, wj.JobId, wj.LineId, strings.Join(wj.Args, " "))

			logger.Debug("ERROR [%v,%v]", dtls.JobId, dtls.Progress.Errored)

		case wj := <-this.FinishedChan:
			dtls := <-this.Details
			dtls.Progress.Finished = 1 + dtls.Progress.Finished
			dtls.LastModified = time.Now().String()
			this.Details <- dtls

			fmt.Fprintf(logFile, "FINISHED %v %v %v %v\n", wj.SubId, wj.JobId, wj.LineId, strings.Join(wj.Args, " "))

			logger.Debug("FINISHED [%v,%v]", dtls.JobId, dtls.Progress.Finished)
		case swj := <-this.SubmittedChan:
			fmt.Fprintf(logFile, "SUBMITTED to %v %v %v %v %v\n", swj.host, swj.wj.SubId, swj.wj.JobId, swj.wj.LineId, strings.Join(swj.wj.Args, " "))

		}

		dtls := this.SniffDetails()
		if dtls.Progress.isComplete() {
			fmt.Fprintln(logFile, "COMPLETED")
			logger.Debug("COMPLETED [%v]", dtls)
			this.SetState(COMPLETE, SUCCESS)
			this.doneChan <- 1
			this.doneChan <- 1
			logger.Debug("COMPLETED [%v]: DONE", dtls.JobId)
			return
		}
	}
}

func (this *Submission) SubmitJobs(jobChan chan *WorkerJob) {
	logger.Debug("SubmitJobs()")

	this.SetState(RUNNING, READY)

	dtls := this.SniffDetails()
	taskId := 0
	for lineId, vals := range this.Tasks {
		logger.Debug("Submitting [%d,%v]", lineId, vals)
		for i := 0; i < vals.Count; i++ {
			select {
			case jobChan <- &WorkerJob{SubId: dtls.JobId, LineId: lineId, JobId: taskId, Args: vals.Args}:
				taskId++
			case <-this.stopChan:
				logger.Printf("submission stopped [%d, %v]", taskId, dtls.JobId)
				return //TODO: add indication that we stopped
			}
		}
	}
	logger.Printf("tasks submitted [%d, %v]", taskId, dtls.JobId)

}

func (this *Submission) WriteCout() {
	dtls := this.SniffDetails()
	logger.Debug("WriteCout(%v)", dtls.JobId)

	var stdOutFile io.WriteCloser = nil
	var err error

	for {
		select {
		case msg := <-this.CoutFileChan:
			if stdOutFile == nil {
				if stdOutFile, err = os.Create(fmt.Sprintf("%v.out.txt", dtls.JobId)); err != nil {
					logger.Warn(err)
				}
				if stdOutFile != nil {
					defer stdOutFile.Close()
				}
			}

			fmt.Fprint(stdOutFile, msg)
		case <-time.After(time.Second):
			//logger.Debug("checking for done: %v", dtls.JobId)
			select {
			case <-this.doneChan:
				logger.Debug("stop chan: %v", dtls.JobId)
				return
			default:
			}

		}
	}
}

func (this *Submission) WriteCerror() {
	dtls := this.SniffDetails()
	logger.Debug("WriteCerror(%v)", dtls.JobId)

	var stdErrFile io.WriteCloser = nil
	var err error

	for {
		select {
		case errmsg := <-this.CerrFileChan:
			if stdErrFile == nil {
				if stdErrFile, err = os.Create(fmt.Sprintf("%v.err.txt", dtls.JobId)); err != nil {
					logger.Warn(err)
				}
				if stdErrFile != nil {
					defer stdErrFile.Close()
				}
			}

			fmt.Fprint(stdErrFile, errmsg)
		case <-time.After(time.Second):
			//logger.Debug("checking for done: %v", dtls.JobId)
			select {
			case <-this.doneChan:
				logger.Debug("stop chan: %v", dtls.JobId)
				return
			default:
			}

		}
	}
}

func (this *Submission) SetState(state string, status string) {
	logger.Debug("SetState(%v,%v):before=%v", state, status, this.SniffDetails())
	x := <-this.Details
	x.State = state
	x.Status = status
	x.LastModified = time.Now().String()
	this.Details <- x
	logger.Debug("SetState(%v,%v):after=%v", state, status, this.SniffDetails())
}

func (this *Submission) UpdateProgress() {
	logger.Debug("UpdateProgress():before=%v", this.SniffDetails())
	x := <-this.Details
	x.Progress.Errored = 1 + x.Progress.Errored
	x.LastModified = time.Now().String()
	this.Details <- x
	logger.Debug("UpdateProgress():after=%v", this.SniffDetails())
}
