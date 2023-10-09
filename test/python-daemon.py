import sys, os, time

def main():
    time.sleep(200)

if __name__ == "__main__":
    # Do the Unix double-fork magic; see Stevens's book "Advanced
    # Programming in the UNIX Environment" (Addison-Wesley) for details
    try:
        pid = os.fork()
        if pid > 0:
            # Exit first parent
            sys.exit(0)
    except OSError as e:
        print("fork #1 failed: %d (%s)" % (
            e.errno, e.strerror), file=sys.stderr)
        sys.exit(1)

    # Decouple from parent environment
    os.setsid()

    # Do second fork
    try:
        pid = os.fork()
        if pid > 0:
            # Exit from second parent; print eventual PID before exiting
            print("Daemon PID %d" % pid)
            sys.exit(0)
    except OSError as e:
        print("fork #2 failed: %d (%s)" % (
            e.errno, e.strerror), file=sys.stderr)
        sys.exit(1)

    # Start the daemon main loop
    main()