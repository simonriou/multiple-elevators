import subprocess
import platform
import time
import os

def open_terminal(command, cwd):
    system = platform.system()

    if system == 'Windows':
        subprocess.Popen(["cmd.exe", "/k", command], cwd=cwd)
    elif system == 'Linux':
        subprocess.Popen(["gnome-terminal", "--", "bash", "-c", f"{command}; exec bash"], cwd=cwd)
    elif system == 'Darwin':
        subprocess.Popen([
            "osascript", "-e",
            f'tell application "Terminal" to do script "cd \\"{cwd}\\" && {command} && exec bash"'
        ])
    else:
        print("Unsupported OS: ", system)

commands = {
    "Windows": {
        "Server": [
            "cd ../binaries && ./SimElevatorServerWindows --port 12120",
            "cd ../binaries && ./SimElevatorServerWindows --port 12121",
            "cd ../binaries && ./SimElevatorServerWindows --port 12122"
        ],
        "Client": [
            "cd ../binaries && ./elevatorClientWindows --port 12120 --id 0 --role Master",
            "cd ../binaries && ./elevatorClientWindows --port 12121 --id 1 --role PrimaryBackup",
            "cd ../binaries && ./elevatorClientWindows --port 12122 --id 2 --role Regular"
        ]
    },
    "Linux": {
        "Server": [
            "cd ../binaries && ./SimElevatorServer --port 12120",
            "cd ../binaries && ./SimElevatorServer --port 12121",
            "cd ../binaries && ./SimElevatorServer --port 12122"
        ],
        "Client": [
            "cd ../binaries && ./elevatorClient --port 12120 --id 0 --role Master",
            "cd ../binaries && ./elevatorClient --port 12121 --id 1 --role PrimaryBackup",
            "cd ../binaries && ./elevatorClient --port 12122 --id 2 --role Regular"
        ]
    },
    "Darwin": {
        "Server": [
            "cd ../binaries && ./SimElevatorServerMacOS --port 12120",
            "cd ../binaries && ./SimElevatorServerMacOS --port 12121",
            "cd ../binaries && ./SimElevatorServerMacOS --port 12122"
        ],
        "Client": [
            "cd ../binaries && ./elevatorClientMacOS --port 12120 --id 0 --role Master",
            "cd ../binaries && ./elevatorClientMacOS --port 12121 --id 1 --role PrimaryBackup",
            "cd ../binaries && ./elevatorClientMacOS --port 12122 --id 2 --role Regular"
        ]
    }
}

current_os = platform.system()
current_dir = os.path.dirname(os.path.abspath(__file__))

if current_os in commands:
    for command in commands[current_os]["Server"]:
        open_terminal(command, current_dir)
        # Wait a bit
        time.sleep(1)

    time.sleep(2)

    for command in commands[current_os]["Client"]:
        open_terminal(command, current_dir)
        time.sleep(1)
else:
    print("Unsupported OS: ", current_os)