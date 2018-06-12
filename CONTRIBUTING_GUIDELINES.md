# How to Contribute to Jaeger

We'd love your help!

Jaeger is [Apache 2.0 licensed](LICENSE) and accepts contributions via GitHub
pull requests. This document outlines some of the conventions on development
workflow, commit message formatting, contact points and other resources to make
it easier to get your contribution accepted.

We gratefully welcome improvements to documentation as well as to code.

# Certificate of Origin

By contributing to this project you agree to the [Developer Certificate of
Origin](https://developercertificate.org/) (DCO). This document was created
by the Linux Kernel community and is a simple statement that you, as a
contributor, have the legal right to make the contribution. See the [DCO](DCO)
file for details.

## Making A Change

*Before making any significant changes, please open an
issue.* Each issue should describe the following:
* Requirement - what kind of business use case are you trying to solve?
* Problem - what in Jaeger blocks you from solving the requirement?
* Proposal - what do you suggest to solve the problem or improve the existing situation?
* Any open questions to address

Discussing your proposed changes ahead of time will make the contribution process smooth for everyone. Once the approach is agreed upon, make your changes and open a pull request (PR). Each PR should describe:
* Which problem it is solving. Normally it should be simply a reference to the corresponding issue, e.g. `Resolves #123`.
* What changes are made to achieve that.

Your pull request is most likely to be accepted if **each commit**:
* Has a [good commit message](https://chris.beams.io/posts/git-commit/). In summary:
    * Separate subject from body with a blank line
    * Limit the subject line to 50 characters
    * Capitalize the subject line
    * Do not end the subject line with a period
    * Use the imperative mood in the subject line
    * Wrap the body at 72 characters
    * Use the body to explain _what_ and _why_ instead of _how_
* Has been signed by the author ([see below](#sign-your-work)).

## License

By contributing your code, you agree to license your contribution under the terms
of the [Apache License](LICENSE).

If you are adding a new file it should have a header like below. In some languages, e.g. Python, you may need to change the comments to start with `#`. The easiest way is to copy the header from one of the existing source files and make sure the year is current and the copyright says "The Jaeger Authors".

```
// Copyright (c) 2018 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
```

## Sign your work

The sign-off is a simple line at the end of the explanation for the
patch, which certifies that you wrote it or otherwise have the right to
pass it on as an open-source patch.  The rules are pretty simple: if you
can certify the below (from
[developercertificate.org](http://developercertificate.org/)):

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.
660 York Street, Suite 102,
San Francisco, CA 94110 USA

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.


Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```

then you just add a line to every git commit message:

    Signed-off-by: Joe Smith <joe@gmail.com>

using your real name (sorry, no pseudonyms or anonymous contributions.)

You can add the sign off when creating the git commit via `git commit -s`.
