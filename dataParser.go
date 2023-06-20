package main

import (
	"container/list"
)

var counter = 0
var callback = make(chan pack)
var queue = list.New()

func balancer(upstream chan pack) {
		for {
			select {
			case newPack := <- upstream:
				if counter < 5 {
					go sentenceParser(newPack, callback)
					counter++
				} else {
					queue.PushBack(newPack)
				}
			// case back := <- callback:
			// 	counter -= back
			// 	if queue.Len() != 0 {
			// 		for counter < 5 {
			// 			frontElement := queue.Front()
			// 			if frontElement == nil {
			// 				break
			// 			}
			// 			go sentenceParser(frontElement.Value.(pack), callback)
			// 			counter++
			// 			queue.Remove(frontElement)
			// 		}
			// 	}
		}
	}
}