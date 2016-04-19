package main

import (
	"fmt"
	"log"

	"github.com/dullgiulio/avantur/store"
)

type projectsAct int

const (
	projectsActPush projectsAct = iota
	projectsActMerge
)

type projectsReq struct {
	act   projectsAct
	build *build
	notif *notif
	bot   *mergebot
}

func newProjectsReq(act projectsAct, b *build, n *notif, bot *mergebot) *projectsReq {
	return &projectsReq{
		act:   act,
		build: b,
		notif: n,
		bot:   bot,
	}
}

type projects struct {
	stages map[string]*build // stage : build
	reqs   chan *projectsReq
}

type dirnotif struct {
	notif *notif
	dir   string
}

type branchDirnotif struct {
	entries map[string]*dirnotif
	name    string
}

func newBranchDirnotif(name string) *branchDirnotif {
	return &branchDirnotif{
		entries: make(map[string]*dirnotif),
		name:    name,
	}
}

func (b *branchDirnotif) add(branch, dir string) error {
	git := newGitcommits()
	log.Printf("[project] getting last commit for branch %s in %s", branch, dir)
	if err := git.last(1, dir); err != nil {
		return fmt.Errorf("cannot detect last commit for branch %s dir %s: %s", branch, dir, err)
	}
	b.entries[branch] = &dirnotif{
		notif: newNotif(b.name, string(git.commits[0].hash), branch),
		dir:   dir,
	}
	return nil
}

func (b branchDirnotif) get(branch string) (*dirnotif, bool) {
	bn, ok := b.entries[branch]
	if ok {
		return bn, true
	}
	return &dirnotif{
		notif: newNotif(b.name, "", branch),
	}, false
}

func newProjects(cf *config, bots mergebots) *projects {
	pjs := &projects{
		stages: make(map[string]*build),
		reqs:   make(chan *projectsReq),
	}
	for proname, procf := range cf.Envs {
		// Start a mergebot for this project
		bot := bots.create(proname, cf)
		go bot.run(pjs)
		// Detect the last commit for each checked-out project
		branchNotif := newBranchDirnotif(proname)
		for branch, dir := range procf.Merges {
			if err := branchNotif.add(branch, dir); err != nil {
				log.Printf("[project] %s: error initializing checked-out project: %s", proname, err)
			}
		}
		// Create builds for already existing static environments
		for _, branch := range procf.Statics {
			bn, notifyMerge := branchNotif.get(branch)
			builds, err := newBuilds(bn.notif, cf)
			if err != nil {
				log.Printf("[project] cannot add existing build %s, branch %s: %s", proname, branch, err)
				continue
			}
			if len(builds) == 0 {
				log.Printf("[project] no static builds to manage for %s, branch %s", proname, branch)
				continue
			}
			for _, b := range builds {
				pjs.stages[b.stage] = b
				go b.run()
				log.Printf("[project] added stage %s tracking %s", b.stage, branch)
			}
			// Notify the merge detector that this is the current build and notif for this directory
			if notifyMerge {
				bot.addCheckout(bn.dir, bn.notif, builds[0])
			}
		}
	}
	go pjs.run()
	return pjs
}

func (p *projects) run() {
	for req := range p.reqs {
		var err error
		switch req.act {
		case projectsActPush:
			err = p.doPush(req)
		case projectsActMerge:
			err = p.doMerge(req)
		}
		if err != nil {
			log.Printf("[project] error processing build action: %s", err)
		}
	}
}

func (p *projects) push(b *build, n *notif, bot *mergebot) {
	p.reqs <- newProjectsReq(projectsActPush, b, n, bot)
}

func (p *projects) merge(b *build, n *notif) {
	p.reqs <- newProjectsReq(projectsActMerge, b, n, nil)
}

// A branch has been pushed: create env or deploy to existing
func (p *projects) doPush(req *projectsReq) error {
	var act store.BuildAct
	if existingBuild, ok := p.stages[req.build.stage]; !ok {
		p.stages[req.build.stage] = req.build
		act = store.BuildActCreate
		go req.build.run()
	} else {
		// If the branch is the same as last seen, act will be treated as Update
		act = store.BuildActChange
		req.build = existingBuild
	}
	req.build.request(act, req.notif)
	req.bot.send(newMergereq(req.notif, req.build))
	return nil
}

func (p *projects) doMerge(req *projectsReq) error {
	stage := req.build.stage
	log.Printf("[project] remove build stage %s", stage)
	build, ok := p.stages[stage]
	if !ok {
		return fmt.Errorf("unknown stage %s merged", stage)
	}
	build.request(store.BuildActDestroy, req.notif)
	build.destroy()
	delete(p.stages, stage)
	return nil
}
