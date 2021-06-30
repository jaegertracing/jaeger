# Jaeger Governance

This document defines governance policies for the Jaeger project.

## Maintainers

Jaeger Maintainers have write access to the Jaeger GitHub repository https://github.com/jaegertracing/jaeger.
They can merge their own patches or patches from others. The current maintainers can be found in [CODEOWNERS](./CODEOWNERS).

This privilege is granted with some expectation of responsibility: maintainers are people who care about the Jaeger project and want to help it grow and improve. A maintainer is not just someone who can make changes, but someone who has demonstrated his or her ability to collaborate with the team, get the most knowledgeable people to review code, contribute high-quality code, and follow through to fix issues (in code or tests).

A maintainer is a contributor to the Jaeger project's success and a citizen helping the project succeed.

## Becoming a Maintainer

To become a maintainer you need to demonstrate the following:

  * commitment to the project
    * participate in discussions, contributions, code reviews for 3 months or more,
    * perform code reviews for 10 non-trivial pull requests,
    * contribute 10 non-trivial pull requests and have them merged into master,
  * ability to write good code,
  * ability to collaborate with the team,
  * understanding of how the team works (policies, processes for testing and code review, etc),
  * understanding of the projects' code base and coding style.

A new maintainer must be proposed by an existing maintainer by sending a message to the
[jaeger-tracing@googlegroups.com](https://groups.google.com/forum/#!forum/jaeger-tracing)
mailing list containing the following information:

  * nominee's first and last name,
  * nominee's email address and GitHub user name,
  * an explanation of why the nominee should be a committer,
  * a list of links to non-trivial pull requests (top 10) authored by the nominee.

Two other maintainers need to second the nomination. If no one objects in 5 working days (U.S.), the nomination is accepted.  If anyone objects or wants more information, the maintainers discuss and usually come to a consensus (within the 5 working days). If issues can't be resolved, there's a simple majority vote among current maintainers.

## Maintainer duties

Maintainers are required to participate in the project, by joining discussions, submitting and reviewing pull requests, answering user questions, among others.

Besides that, we have one concrete activity in which maintainers have to engage from time to time: releasing new versions of Jaeger. This process ideally takes only a couple of hours, but requires coordination on different fronts. Even though the process is well documented, it is not without eventual glitches, so, each release needs a "Release Manager". How it works is described in the [RELEASE.md](RELEASE.md) file.

Maintainers are also encouraged to speak about Jaeger at conferences, especially KubeCon+CloudNativeCon which happens twice a year. This event has a "maintainer track", in which maintainers can give an introduction and/or a deep dive about their projects. The Jaeger project has always participated since it became part of the CNCF.

## Changes in Maintainership

We do not expect anyone to make a permanent commitment to be a Jaeger maintainer forever. After all, circumstances change,
people get new jobs, new interests, and may not be able to continue contributing to the project. At the same time, we need
to keep the list of maintainers current in order to have effective governance. People may be removed from the current list
of maintainers via one of the following ways:
  * They can resign
  * If they stop contributing to the project for a period of 6 months or more
  * By a 2/3 majority vote by maintainers

Former maintainers can be reinstated to full maintainer status through the same process of
[Becoming a Maintainer](#becoming-a-maintainer) as first-time nominees.

## Emeritus Maintainers

Former maintainers are recognized with an honorary _Emeritus Maintainer_ status, and have their names permanently
listed in the README as a form of gratitude for their contributions.

## GitHub Project Administration

Maintainers will be added to the GitHub @jaegertracing/jaeger-maintainers team, and made a GitHub maintainer of that team.
They will be given write permission to the Jaeger GitHub repository https://github.com/jaegertracing/jaeger.

## Changes in Governance

All changes in Governance require a 2/3 majority vote by maintainers.

## Other Changes

Unless specified above, all other changes to the project require a 2/3 majority vote by maintainers.
Additionally, any maintainer may request that any change require a 2/3 majority vote by maintainers.
