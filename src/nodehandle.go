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
	"json"
	"strconv"
)

type NodeHandle struct {
	NodeId        string
	Uri           string
	Hostname      string
	Master        *Master
	Con           Connection
	MaxJobs       chan int
	Running       chan int
	BroadcastChan chan *WorkerMessage
}

func NewNodeHandle(n *Connection, m *Master) *NodeHandle {
	vlog("NewNodeHandle(%v)", n.isWorker)
	con := *n
	id := UniqueId()
	nh := NodeHandle{NodeId: id,
		Uri:           "/nodes/" + id,
		Hostname:      con.Socket.LocalAddr().String(),
		Master:        m,
		Con:           con,
		MaxJobs:       make(chan int, 1),
		Running:       make(chan int, 1),
		BroadcastChan: make(chan *WorkerMessage, 0)}

	//wait for worker handshake TODO: should this be in monitor???
	nh.Running <- 0
	msg := <-nh.Con.InChan

	if msg.Type == HELLO {
		val, err := strconv.Atoi(msg.Body)
		if err != nil {
			logger.Warn(err)
			return nil
		}
		nh.MaxJobs <- val
	} else {
		vlog("%v didn't say hello as first message.", nh.Hostname)
		return nil
	}
	vlog("%v says hello and asks for %v jobs.", nh.Hostname, msg.Body)
	return &nh
}

func (nh *NodeHandle) Stats() (processes int, running int) {
	vlog("Stats()")
	running = <-nh.Running
	nh.Running <- running
	processes = <-nh.MaxJobs
	nh.MaxJobs <- processes
	return
}

func (nh *NodeHandle) ReSize(newMaxJobs int) {
	vlog("ReSize(%d)", newMaxJobs)
	<-nh.MaxJobs
	nh.MaxJobs <- newMaxJobs
}

// turns job into JSON and send to connections outbox. Seems to sleep or deadlock if left alone to long so the worker checks-in every 60 seconds.
func (nh *NodeHandle) SendJob(j *WorkerJob) {
	job := *j
	vlog("SendJob(%v): %v", job, nh.Hostname)
	jobjson, err := json.Marshal(job)
	if err != nil {
		logger.Warn(err)
	}
	msg := WorkerMessage{Type: START, Body: string(jobjson)}
	nh.Con.OutChan <- msg
}

func (nh *NodeHandle) Monitor() {
	vlog("Monitor(): [%v]", nh.Hostname)
	//control loop
	for {
		processes, running := nh.Stats()
		vlog("Monitor(): [%v %d %d]", nh.Hostname, processes, running)

		switch {
		case running < processes:
			vlog("Monitor(): %v running %v. Waiting for job or message.", nh.Hostname, running)
			select {
			case bcMsg := <-nh.BroadcastChan:
				vlog("Monitor(): %v broadcasting message %v", nh.Hostname, *bcMsg)
				nh.Con.OutChan <- *bcMsg
			case job := <-nh.Master.jobChan:
				nh.SendJob(job)
				running := <-nh.Running
				nh.Running <- running + 1
				vlog("Monitor(): %v assigned job, %v running", nh.Hostname, running)
			case msg := <-nh.Con.InChan:
				nh.HandleWorkerMessage(&msg)
			}
		default:
			vlog("Monitor(): %v running %v. Waiting for message.", nh.Hostname, running)
			select {
			case bcMsg := <-nh.BroadcastChan:
				vlog("Monitor(): %v broadcast message %v", nh.Hostname, *bcMsg)
				nh.Con.OutChan <- *bcMsg
			case msg := <-nh.Con.InChan:
				nh.HandleWorkerMessage(&msg)
			}
		}
	}
}

//handle worker messages and updates the value in nh.Running if appropriate
func (nh *NodeHandle) HandleWorkerMessage(msg *WorkerMessage) {
	vlog("msg from %v", nh.Hostname)
	switch msg.Type {
	default:
	case CHECKIN:
		vlog("CHECKIN %v", nh.Hostname)
	case COUT:
		vlog("COUT %v", nh.Hostname)
		nh.Master.GetSub(msg.SubId).CoutFileChan <- msg.Body
	case CERROR:
		vlog("CERROR %v", nh.Hostname)
		nh.Master.GetSub(msg.SubId).CerrFileChan <- msg.Body
	case JOBFINISHED:
		vlog("JOBFINISHED %v", nh.Hostname)
		running := <-nh.Running
		nh.Running <- running - 1
		vlog("JOBFINISHED %v job finished: %v running: %v", nh.Hostname, msg.Body, running)
		nh.Master.GetSub(msg.SubId).FinishedChan <- NewWorkerJob(msg.Body)
		logger.Printf("JOBFINISHED %v finished: %v, running: %v", nh.Hostname, msg.Body, running)
	case JOBERROR:
		vlog("JOBERROR %v", nh.Hostname)
		running := <-nh.Running
		nh.Running <- running - 1
		vlog("JOBERROR %v %v running:%v", nh.Hostname, msg.Body, running)
		nh.Master.GetSub(msg.SubId).ErrorChan <- NewWorkerJob(msg.Body)
		vlog("JOBERROR %v finished sent to Sub: %v running:%v", nh.Hostname, msg.Body, running)
	}
	vlog("%v msg handled", nh.Hostname)
}
