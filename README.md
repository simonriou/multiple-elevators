TTK4145 - Group 38 - Multiple Elevators Project
======================

Latest update: March, 28th

# Unstable / missing features
- Packet loss is breaking the elevator client as soon as 40-50% of the information is lost. The peers disconnect too often for the peer update channel to keep track of it in our current configuration. In addition, hall button presses and light updates are not registered.
- Elevators that experience power loss (no script termination, only motor power loss) **and that don't have any pending order** are not flaged as inactive.
- Obstruction while having hall orders does not redistribute them. Instead, the elevator handles them whenever it is able to close its doors.
- When an elevator experiences power loss and that another one takes the hall order that it had, the Hall Light turn off is delayed until the first elevator gets back on and goes for it.

# Usage
Here is a detailed explaination on how to use the multiple elevators repository. The binary for the client can be found in the releases section.
## First launch
Each elevator client must have a dedicated elevator server. One 'elevator' is thus composed of either a simulator (`simElevatorServer`, `simElevatorServer.exe`, `simElevatorServerMacOS`) OR hardware server (`elevatorServer`) AND of a client (`elevatorClient`, `elevatorClientMacOS` or `elevatorClientWindows.exe`, see releases section). In the future, 'server' will refer to either the hardware server or the simulator. The recommended process to launch multiple is the following:

- Start with executing **every server**. The first variables declared in `globalVariables.go` must be adjusted to fit the number of elevators you will run. You must also specify the port on which the server and the client will communicate. Each pair of elevator / server must operate on a **different port**. They all must be **different than the ports defined** inside of `globalVariables.go`. An example would be
    - First pair of elevator / server operating on `12120`
    - Second one on `12121`
    - Third one on `12122`

    The correct syntax for launching a server is (e.g. in the case of a simulator running on a MacOS machine)

    ```bash
    ./SimElevatorServerMacOS --port=12120
    ```
- Once all the servers are started, launch every elevator client. Although it is recommended to start the *Master* elevator first, the script will work no matter which elevator starts first. In case of a use with two elevators only, we **must have a *Master* and a *PrimaryBackup* at all times**. Two elevators **cannot have the same role**. Each elevator must have an **unique** ID which **must be a positive integer** the IDs of the elevator must be **consecutive integers starting at 0**. Upon launching a client, three parameters must be specified:
    - The **port** on which it will communicate with the server
    - The **ID** of the elevator, a positive integer, **unique**, consecutives, starting at 0.
    - Its **role**, a string, **unique**, which can be **[*Regular*, *Master* or *PrimaryBackup*]**. The roles **are case-sensitive**.

    Here is an example of a correct syntax for the launch of elevator ID 0, role Master on port 12120:

    ```bash
    ./elevatorClient --port=12120 --id=0 --role=Master
    ```

    Note that the command must be run in the same directory as the binary, and that the order in which the parameters are passed is of no importance. Alternatively, you can build the project directly from the `.src/` directory, using `go run .` followed by the same set of arguments.

## Re-launch after shutdown (important)
In case of the restart of an elevator after it went down, there is something to consider: whenever an elevator goes down, the two remaining ones change roles so that there always are *Master* and *PrimaryBackup* elevators at all times. This means that **if a *Master* or a *PrimaryBackup* goes down, we must restart it as a *Regular*** elevator, because another elevator will have taken his role by then. However **its ID must remain unchanged**. This is only affected for restarts after force termination of a script (using `Ctrl+C`). In case of a network or power loss (unplugging the respective cable), there is no need to specify a new role to the elevator.

# File Organisation

## Main file
`main.go` contains the core features of an elevator. It basically calls routines to handle every event that could happen during the elevator's work.

## Routines
These routines are inside `routines.go`. Their name is pretty self-explainatory, but details on the logic can be found inside the [Logic](#logic) section.

## Utility file
`util.go` contains a whole lot of utility function that are used at some point throughout the code. It is not very relevant to describe each one of them, as they mainly perform basic operations that help keep the logic clear.

## Types file
`types.go` is the place where all the custom structures and types are declared.

## Global variables file
`globalVariables.go` contains a list of variables and constants that are typically used by multiple go files and routines. The vast majority of them comes with their associated mutex to ensure good behaviour when simultaneous update. Below is an overview of the role of the most important ones.

### Global values description
- `role` is a string containing the role of the elevator (`Master`, `PrimaryBackup` or `Regular`). This is not a constant as the roles change whenever an elevator is unable to attend to new orders.
- `elevatorOrders` is the list of orders that **this** elevator has to attend to.
- `posArray` is a positonal array that is updated each time an elevator reaches or leaves a floor. It is used in the sorting of the orders.
- `ableToCloseDoors` is a global boolean that is triggered with the obstruction button.
- `latestState` is the variable that is used to update the state of the elevator.
- `activeElevators` is an array containing the ids of the elevator that are able to attend to new orders. It is being sorted everytime it is updated.
- `backupStates` is the variable used by the backup elevator to store the latest states, at all times.

## Initialization file
`initialization.go` contains the functions that are used during the launch of an elevator.

## Order handling file
`handleOrders.go` contains all the logic related to the handling of orders (essentially sorting orders). See more in [Logic](#Logic)

## Communications file
`communications.go` contains the functions required to ensure the communication between the elevators, as well as the cost function.
### File overview
- `MasterRoutine` contains all the tasks that are specific to the master elevator. This includes the initialization of channels, as well as handling new orders, assign them to elevators and send states updates to the backup elevator.
- `PrimaryBackupRoutine` does the same thing but for the primary backup's tasks. This essentially is updating the global variable containing the save of the states.
- `calculateCost` is the cost function. Its role is to assign a cost to an elevator taking an order. It is based on the distance between the elevator and the order, and then tweaks the cost depending on the behaviour and direction of the elevator.

# Logic

## Overall procedure
The system is composed of **three elevators**, each one with a different role: a **Master** elevator, a **Primary Backup** elevator, and a **Regular** one.
1. <u>New button press received</u>
    - Hall order: the elevator that got the order sends it to the master elevator (using the `hallBtnTx` channel).
    - Cab order: the elevator adds the order to its local array of orders to attend to (`elevatorOrders`). It then sorts its local orders and sends its new state to the master. The first element of its local orders is then set as the order to attend to now (if there is any).
2. <u>New order received from the master</u> - All elevators get the new order message, which contains the order to attend to as well as the id of the elevator that should take it. They all check if they the right elevator, and if so, they add the order to their local array, sort it, send the new state to the master and set the first element of the array to be the current order (if any).
3. <u>New floor reached</u> - The `lastFloor` variable is updated, as well as the state of the elevator, which is then sent to the master.
4. <u>Stop button update</u>
    - Pressed: The direction of the elevator is set to `stop`, and it is removed from the `activeElevators` array. All of its local orders (i.e. the `elevatorOrders` array) are then re-assigned to other elevators (i.e. sent to the `hallBtnTx` channel again).
    - Unpressed: The elevator is added back to the `activeElevators` array. Its old cab orders are retrieved thanks to the primary backup which has them stored. Its hall orders were re-assigned when it went down.
5. <u>Obstruction switch</u>
    - Off to On: The `ableToCloseDoors` global variable is set to `false`
    - On to Off: The `ableToCloseDoors` global variable is set to `true`
6. <u>Network peer update</u> - The `Transmiter` and `Receiver` functions from `network/peers` were tweaked so that a peer sends both its role and id. The data received by the `peerUpdateCh` is thus converted from a string to a structure.
    - New peer: We add the peer back to the `activeElevators` array.
    - Lost peer: It is assumed that **only one elevator can be down at a time**. We begin by removing the lost elevator from `activeElevators`. Then we handle the role changes. If the *Master* goes down, then *PrimaryBackup* becomes *Master* and *Regular* becomes *PrimaryBackup*. If the *PrimaryBackup* goes down, then *Regular* becomes *PrimaryBackup*. We also launch the corresponding routines after assigning the new roles. Finally, we re-assign the hall orders of the lost elevators (same logic as the stop button case).

On top of all of that, the master is at all times sending its backup states to all the slaves (who update their own state based on this information), and each slave periodically sends its own state to the master, who update its backup states with it. This is supposed to protect the elevators from packet loss.