// Copyright 2015 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package drive

import (
	"time"
)

func (g *Commands) Touch() (err error) {
	// Arbitrary value for rate limiter
	throttle := time.Tick(1e9 / 10)

	chanMap := map[int]chan *keyValue{}

	for i, relToRootPath := range g.opts.Sources {
		chanMap[i] = g.touch(relToRootPath, "")
		<-throttle
	}

	spin := g.playabler()
	spin.play()

	for {
		if len(chanMap) < 1 {
			break
		}
		// Find the channel that has results
		for key, kvChan := range chanMap {
			select {
			case kv := <-kvChan:
				if kv == nil { // Sentinel emitted
					delete(chanMap, key)
					continue
				}
				if kv.value != nil {
					g.log.LogErrf("touch: %s %v\n", kv.key, kv.value.(error))
				}
			default:
			}
		}
	}
	spin.stop()
	return
}

func (g *Commands) TouchByMatch() (err error) {
	matches, err := g.rem.FindMatches(g.opts.Path, g.opts.Sources, false)
	if err != nil {
		return err
	}

	throttle := time.Tick(1e9 / 10)
	chanMap := map[int]chan *keyValue{}

	i := 0
	for match := range matches {
		if match == nil {
			continue
		}

		chanMap[i] = g.touch(g.opts.Path+"/"+match.Name, match.Id)
		<-throttle
		i += 1
	}

	spin := g.playabler()
	spin.play()

	for {
		if len(chanMap) < 1 {
			break
		}
		// Find the channel that has results
		for key, kvChan := range chanMap {
			select {
			case kv := <-kvChan:
				if kv == nil { // Sentinel emitted
					delete(chanMap, key)
					continue
				}
				if kv.value != nil {
					g.log.LogErrf("touch: %s %v\n", kv.key, kv.value.(error))
				}
			default:
			}
		}
	}
	spin.stop()
	return
}

func (g *Commands) touch(relToRootPath, fileId string) chan *keyValue {
	fileChan := make(chan *keyValue)
	go func() {
		kv := &keyValue{
			key: relToRootPath,
		}

		defer func() {
			fileChan <- kv
			fileChan <- nil
			close(fileChan)
		}()

		f, arg := g.rem.Touch, fileId
		if fileId == "" {
			f, arg = g.touchByPath, relToRootPath
		}
		file, err := f(arg)

		if err != nil {
			kv.value = err
			return
		}

		if true { // TODO: Print this out if verbosity is set
			g.log.Logf("%s: %v\n", relToRootPath, file.ModTime)
		}
		if g.opts.Recursive && file.IsDir {
			childResults := make(chan chan *keyValue)
			go func() {
				// Arbitrary value for rate limiter
				throttle := time.Tick(1e9 * 2)
				childrenChan := g.rem.findByParentIdRaw(file.Id, false, g.opts.Hidden)
				for child := range childrenChan {
					childResults <- g.touch(relToRootPath+"/"+child.Name, child.Id)
					<-throttle
				}
				close(childResults)
			}()

			for childChan := range childResults {
				for childFile := range childChan {
					fileChan <- childFile
				}
			}
		}
	}()
	return fileChan
}

func (g *Commands) touchByPath(relToRootPath string) (*File, error) {
	file, err := g.rem.FindByPath(relToRootPath)
	if err != nil {
		return nil, err
	}
	if file == nil {
		return nil, ErrPathNotExists
	}
	return g.rem.Touch(file.Id)
}
