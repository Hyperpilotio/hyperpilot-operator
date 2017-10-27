package operator

import (
	"database/sql"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"log"
)

type EventType int

const (
	ADD    EventType = 1 << iota
	UPDATE EventType = 2
	DELETE EventType = 4
)

type ResourceEvent struct {
	Event_type EventType
}

type Event interface {
	GetType() EventType
	UpdateGlobalStatus()
}

type NodeEvent struct {
	ResourceEvent
	Cur *v1.Node
	Old *v1.Node
}

func (r *NodeEvent) UpdateGlobalStatus() {

	db, err := sql.Open("ramsql", K8SEVENT)
	defer db.Close()
	if err != nil {
		log.Fatalf("sql.Open : Error : %s\n", err)
	}
	if r.Event_type == ADD {
		stmt, err := db.Prepare("INSERT INTO node(name, internal_ip,external_ip) VALUES(?,?,?)")
		if err != nil {
			log.Fatal(err)
		}

		_, err = stmt.Exec(r.Cur.Name, r.Cur.Status.Addresses[0].Address, r.Cur.Status.Addresses[1].Address)

		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Insert Node {%s}, INTERNAL_IP {%s}, EXTERNAL_IP {%s}. ",
			r.Cur.Name, r.Cur.Status.Addresses[0].Address, r.Cur.Status.Addresses[1].Address)
	}

	//TODO
	if r.Event_type == UPDATE {
	}

	//TODO
	if r.Event_type == DELETE {
	}
}

func (r *NodeEvent) GetType() EventType {
	return r.Event_type
}

type PodEvent struct {
	ResourceEvent
	Cur *v1.Pod
	Old *v1.Pod
}

func (r *PodEvent) UpdateGlobalStatus() {
	db, err := sql.Open("ramsql", K8SEVENT)
	defer db.Close()
	if err != nil {
		log.Fatalf("sql.Open : Error : %s\n", err)
	}

	// ADD event with running means the pod already run before operator
	if r.Event_type == ADD && r.Cur.Status.Phase == "Running" {
		stmt, err := db.Prepare("INSERT INTO pod(name, status, node) VALUES(?,?,?)")
		if err != nil {
			log.Fatal(err)
		}

		_, err = stmt.Exec(r.Cur.Name, r.Cur.Status.Phase, r.Cur.Spec.NodeName)

		if err != nil {
			log.Fatal(err)
		}

		log.Printf("Insert Existing Pod {%s}, Status {%s}, node {%s}. ",
			r.Cur.Name, r.Cur.Status.Phase, r.Cur.Spec.NodeName)
	}

	//delete pod info when delete event happens
	if r.Event_type == DELETE {
		stmt, err := db.Prepare("DELETE from pod where name = ?")
		if err != nil {
			log.Fatal(err)
		}
		_, err = stmt.Exec(r.Cur.Name)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Delete Pod {%s}", r.Cur.Name)
	}

	// node info is available until pod is in running state
	if r.Event_type == UPDATE {
		if r.Old.Status.Phase == "Pending" && r.Cur.Status.Phase == "Running" {
			stmt, err := db.Prepare("INSERT INTO pod(name, status, node) VALUES(?,?,?)")
			if err != nil {
				log.Fatal(err)
			}

			_, err = stmt.Exec(r.Cur.Name, r.Cur.Status.Phase, r.Cur.Spec.NodeName)

			if err != nil {
				log.Fatal(err)
			}

			log.Printf("Insert NEW Pod {%s}, Status {%s}, node {%s}. ",
				r.Cur.Name, r.Cur.Status.Phase, r.Cur.Spec.NodeName)
		}
	}
}

func (r *PodEvent) GetType() EventType {
	return r.Event_type
}

type DeploymentEvent struct {
	ResourceEvent
	Old *v1beta1.Deployment
	Cur *v1beta1.Deployment
}

func (r *DeploymentEvent) GetType() EventType {
	return r.Event_type
}

func (r *DeploymentEvent) UpdateGlobalStatus() {
	//TODO
	if r.Event_type == ADD {
	}

	//TODO
	if r.Event_type == DELETE {
	}

	//TODO
	if r.Event_type == UPDATE {
	}

}

type DaemonSetEvent struct {
	ResourceEvent
	Old *v1beta1.DaemonSet
	Cur *v1beta1.DaemonSet
}

func (r *DaemonSetEvent) GetType() EventType {
	return r.Event_type
}

func (r *DaemonSetEvent) UpdateGlobalStatus() {
	//TODO
	if r.Event_type == ADD {
	}

	//TODO
	if r.Event_type == DELETE {
	}

	//TODO
	if r.Event_type == UPDATE {
	}
}
