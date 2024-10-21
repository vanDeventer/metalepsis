/*******************************************************************************
 * Copyright (c) 2023 Jan van Deventer
 *
 * All rights reserved. This program and the accompanying materials
 * are made available under the terms of the Eclipse Public License v2.0
 * which accompanies this distribution, and is available at
 * http://www.eclipse.org/legal/epl-2.0/
 *
 * Contributors:
 *   Jan A. van Deventer, Luleå - initial implementation
 *   Thomas Hedeler, Hamburg - initial implementation
 ***************************************************************************SDG*/

package main

import (
	"container/heap"
	"time"
)

//*********************Expired services cleaning scheduler*********************

// cleaningTask holds the time for the next time a service is due to expire
type cleaningTask struct {
	Deadline time.Time // the time when job has to be executed
	Job      func()    // call to check expiration of a record
	Id       int       // the job Id is the record id and is used to remove a scheduled task
}

// cleaningQueue the list of schedlued check on service expiration
type cleaningQueue []*cleaningTask

// Len returns the length of the service list and their expiration time
func (cq cleaningQueue) Len() int { return len(cq) }

// Less checks if a task is due sooner than another
func (cq cleaningQueue) Less(i, j int) bool {
	return cq[i].Deadline.Before(cq[j].Deadline)
}

// Swap exchanges the order of task if one is due before the other
func (cq cleaningQueue) Swap(i, j int) {
	cq[i], cq[j] = cq[j], cq[i]
}

// Push adds a task to the task list or queue
func (cq *cleaningQueue) Push(x interface{}) {
	task := x.(*cleaningTask)
	*cq = append(*cq, task)
}

// Pop remove a task from the cleaning queue
func (cq *cleaningQueue) Pop() interface{} {
	old := *cq
	n := len(old)
	task := old[n-1]
	*cq = old[0 : n-1]
	return task
}

// Scheduler struct type with the list and two channels
type Scheduler struct {
	taskQueue  cleaningQueue
	taskStream chan *cleaningTask
	stopChan   chan struct{}
}

// NewScheduler creates a new scheduler
func NewScheduler() *Scheduler {
	return &Scheduler{
		taskStream: make(chan *cleaningTask),
		stopChan:   make(chan struct{}),
	}
}

// AddTask adds a task to the queue with its deadline
func (s *Scheduler) AddTask(deadline time.Time, job func(), id int) {
	task := &cleaningTask{
		Deadline: deadline,
		Job:      job,
		Id:       id,
	}
	s.taskStream <- task
}

// RemoveTask removes a scheduled task
func (s *Scheduler) RemoveTask(id int) bool {
	// Search for the task with the given Id
	for i, task := range s.taskQueue {
		if task.Id == id {
			// Remove the task from the queue
			s.taskQueue = append(s.taskQueue[:i], s.taskQueue[i+1:]...)
			heap.Init(&s.taskQueue) // Reinitialize the heap
			return true             // Return true indicating the task was found and removed
		}
	}
	return false // Return false if the task wasn't found
}

// run is the  goroutine that cleans up expired services by continuously checking that end of validity of services
func (s *Scheduler) run() {
	var timer *time.Timer
	defer s.Stop()
	for {
		if len(s.taskQueue) > 0 {
			nextTask := s.taskQueue[0]
			if timer == nil {
				timer = time.NewTimer(time.Until(nextTask.Deadline))
			} else {
				timer.Reset(time.Until(nextTask.Deadline))
			}
		}

		time.Sleep(10 * time.Millisecond) // this is used to reduce CPU consumption otherwise the go routine is a "short circuit" with no resistance

		select {
		case task := <-s.taskStream:
			heap.Push(&s.taskQueue, task)
			if timer == nil {
				timer = time.NewTimer(time.Until(task.Deadline))
			} else {
				timer.Reset(time.Until(task.Deadline))
			}
		case <-func() <-chan time.Time {
			if timer != nil {
				return timer.C
			}
			return nil
		}():
			task := heap.Pop(&s.taskQueue).(*cleaningTask)
			go task.Job()
		case <-s.stopChan:
			if timer != nil {
				timer.Stop()
			}
			return
		}
	}
}

// Stop terminnates the scheduler
func (s *Scheduler) Stop() {
	s.stopChan <- struct{}{}
}
