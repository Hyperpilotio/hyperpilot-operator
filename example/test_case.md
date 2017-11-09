## Case 1: 
    1. 3 snaps are running, then start operator
    2. TEST: then create resource worker
    3. CHECK: tasks are created on snap 

## Case 2: 
    1. 3 Snape are running, 3 resource worker are running, then Start operator
    2. TEST: delete 3 resource worker
    3. CHECK: tasks are deleted from snap

## Case 3: 
    1. 3 sanps are running,  start operator and the resource worker
    2. TEST: delete all pods and restart on another node
    3. CHECK: task is deleted first and then  add 

## Case 4: 
    1. 3 Snap are running , 1 resource worker running,  start operator
    2.  TEST: delete pod

## Case 5:
    1. Snap not running, operator start first , then start 3 snap daemonset
    2. TEST: AFTER plugin load , then start 3 resource worker
    3. CHECK: tasks are created on snap 


## Case 6:
    1. Snap not running, operator start first,  ten start 3 snap daemonset
    2. TEST: BEFORE  plugin load complete , then start 3 resource worker
    3. CHECK: tasks are created on snap until plugin loaded


## Case 7:
    1. Snap are running ,  start operator, start resource-worker, stop operator, delete resource worker
    2. TEST: start operator
    3. CHECK: tasks are deleted  

## Case 8:
    1. Snap are running ,  start operator, start resource-worker
    2. TEST: 1 snap restart
    3. CHECK:  task is added to restart snap


## Case 9 :
    1. Snap are running ,  start operator, start resource-worker
    2. 1 snap restart
    3. TEST: remove resource worker
    3. CHECK : all tasks  are  removed 
    
   
## Case 10:
    1. Snap are running ,  start operator, no resource-worker
    2. TEST: when 1 snap restart, create 3 resource worker
    3. CHECK: 2 tasks are add immediately, 1 tasks added after snap plugin load