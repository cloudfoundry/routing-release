## locket

**Note**: This repository should be imported as `code.cloudfoundry.org/locket`.

Locket is a distributed locking service and client libraries to integrate with the service

## Reporting issues and requesting features

Please report all issues and feature requests in [cloudfoundry/diego-release](https://github.com/cloudfoundry/diego-release/issues).

### Server

Diego Client for Setting/Fetching Locks and Presence

### Client

There is currently one valid client
1. the [locket client](https://godoc.org/code.cloudfoundry.org/locket/lock#NewLockRunner) which can be used with the locket service.

A general overview of the Locket API can be found [here](doc).
You can learn more about Diego and its components at [diego-design-notes](https://github.com/cloudfoundry/diego-design-notes).
