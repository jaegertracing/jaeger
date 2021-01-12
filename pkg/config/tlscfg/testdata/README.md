# Example Certificate Authority and Certificate creation for testing

The PEM files located in this directory are used by unit tests in this package.

To generate and update the PEM files in this directory, run the following from the project root:

    make certs

To only generate the PEM files without copying them to this directory:

    make certs-dryrun
    
The location of the generated PEM files will be printed to STDOUT like so:

    # Dry-run complete. Generated files can be found in /var/folders/3p/yms48z2s6v7c8fy2m_1481g00000gn/T/certificates.p7pFHXpy
