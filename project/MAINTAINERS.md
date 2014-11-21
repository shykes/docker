# The Docker Maintainer manual

## Introduction

Dear maintainer. Thank you for investing the time and energy to help
make Docker as useful as possible. Maintaining a project is difficult,
sometimes unrewarding work. Sure, you will get to contribute cool
features to the project. But most of your time will be spent reviewing,
cleaning up, documenting, answering questions, and justifying design
decisions - while everyone has all the fun! But remember - the quality
of the maintainers' work is what distinguishes the good projects from
the great. So please be proud of your work, even the unglamourous parts,
and encourage a culture of appreciation and respect for *every* aspect
of improving the project - not just the hot new features.

This document is a manual for maintainers old and new. It explains what
is expected of maintainers, how they should work, and what tools are
available to them.

This is a living document - if you see something out of date or missing,
speak up!

## What is a maintainer?

There are different types of maintainers, with different responsibilities, but
all maintainers have 3 things in common:

* 1) They share responsibility in the project's success.
* 2) They have made a long-term, recurring time investment to improve the project.
* 3) They spend that time doing whatever needs to be done, not necessarily what
is the most interesting or fun.

Maintainers are often under-appreciated, because their work is harder to appreciate.
It's easy to appreciate a really cool and technically advanced feature. It's harder
to appreciate the absence of bugs, the slow but steady improvement in stability,
or the reliability of a release process. But those things distinguish a good
project from a great one.

## How are decisions made?

Short answer: EVERYTHING IS A PULL REQUEST.

Docker is an open-source project with an open design philosophy. This
means that the repository is the source of truth for EVERY aspect of the
project, including its philosophy, design, road map, and APIs. *If it's
part of the project, it's in the repo. If it's in the repo, it's part of
the project.*

As a result, all decisions can be expressed as changes to the
repository. An implementation change is a change to the source code. An
API change is a change to the API specification. A philosophy change is
a change to the philosophy manifesto, and so on.

All decisions affecting Docker, big and small, follow the same 3 steps:

* Step 1: Open a pull request. Anyone can do this.

* Step 2: Discuss the pull request. Anyone can do this.

* Step 3: Merge or refuse the pull request. Who does this depends on the nature
of the pull request and which areas of the project it affects. See *review flow*
for details.

## Review flow

Pull requests should be processed according to the following flow:

* For each subsystem affected by the change, the maintainers of the subsystem must approve or refuse it.
It is the responsibility of the subsystem maintainers to process patches affecting them in a timely
manner.

* If the change affects areas of the code which are not part of a subsystem,
or if subsystem maintainers are unable to reach a timely decision, it must be approved by 
the core maintainers.

* If the change affects the UI or public APIs, or if it represents a major change in architecture,
the architects must approve or refuse it.

* If the change affects the operations of the project, it must be approved or rejected by
the relevant operators.

* If the change affects the governance, philosophy, goals or principles of the project,
it must be approved by BDFL.


## Who decides what?

Because Docker is such a large and active project, it's important for everyone to know
who is responsible for deciding what. That is determined by a precise set of rules.

* For every *decision* in the project, the rules should designate, in a deterministic way,
who should *decide*.

* For every *problem* in the project, the rules should designate, in a deterministic way,
who should be responsible for *fixing* it.

* For every *question* in the project, the rules should designate, in a deterministic way,
who should be expected to have the *answer*.

Currently the project is organized in 3 groups of people: the BDFL, the core, and the subsystems.

### The Benevolent dictator for life (BDFL)

Docker follows the timeless, highly efficient and totally unfair system
known as [Benevolent dictator for
life](http://en.wikipedia.org/wiki/Benevolent_Dictator_for_Life), with
yours truly, Solomon Hykes, in the role of BDFL. This means that all
decisions are made, by default, by Solomon. Since making every decision
myself would be highly un-scalable, in practice decisions are spread
across multiple maintainers.

Ideally, the BDFL role is like the Queen of England: awesome crown, but not
an actual operational role day-to-day. The real job of a BDFL is to NEVER GO AWAY.
Every other rule can change, perhaps drastically so, but the BDFL will always
be there, preserving the philosophy and principles of the project, and keeping
ultimate authority over its fate. This gives us great flexibility in experimenting
with various governance models, knowing that we can always press the "reset" button
without fear of fragmentation or deadlock. See the US congress for a counter-example.

BDFL daily routine:

* Is the project governance stuck in a deadlock or irreversibly fragmented?
	* If yes: refactor the project governance
* Are there issues or conflicts escalated by core?
	* If yes: resolve them
* Go back to polishing that crown.


### Architects

The Architects are responsible for the overall integrity of the technical architecture
across all subsystems, and the consistency of APIs and UI.

Changes to UI, public APIs and overall architecture (for example a plugin system) must
be approved by the architects.

Architect daily routine:

* Are there design proposals waiting for review?
	* If yes: provide feedback and make a decision
* Are there unanswered questions on the architecture and design of the project?
	* If yes: answer them and update the relevant documentation

Chief Architect: Solomon Hykes <solomon@docker.com>


### Core Maintainers

The Core maintainers are the ghostbusters of the project: when there's a problem others
can't solve, they show up and fix it with bizarre devices and weaponry.
They have final say on technical implementation and coding style.
They are ultimately responsible for quality in all its forms: usability polish,
bugfixes, performance, stability, etc. When ownership  can cleanly be passed to
a subsystem, they are responsible for doing so and holding the
subsystem maintainers accountable. If ownership is unclear, they are the de facto owners.

For each release (including minor releases), a "release captain" is assigned from the
pool of core maintainers. Rotation is encouraged across all maintainers, to ensure
the release process is clear and up-to-date.

Chief Maintainer: Michael Crosby <mike@docker.com>

Active core maintainers:

* Cristian Staretu <cristian@docker.com>
* Tianon Gravi <admwiggin@gmail.com>
* Erik Hollensbe <erik@docker.com>
* Victor Vieux <vieux@docker.com>
* Tibor Vass <tibor@docker.com>
* Jessie Frazelle <jessie@docker.com>

Core maintainers in training:

* Arnaud Porterie <arnaud@docker.com>
* Vishnu Kannan <vishnuk@google.com>
* Vincent Batts <vbatts@redhat.com>

It is common for core maintainers to "branch out" to join or start a subsystem.

### Operators

The operators make sure the trains run on time. They are responsible for overall operations
of the project. This includes facilitating communication between all the participants; helping
newcomers get involved and become successful contributors and maintainers; tracking the schedule
of releases; managing the relationship with downstream distributions and upstream dependencies;
define measures of success for the project and measure progress; Devise and implement tools and
processes which make contributors and maintainers happier and more efficient.

Chief Operator: *we are currently looking for a chief operator*.

Security Operator: Eric Windisch <eric@windisch.us>

	*The security operator runs the process of receiving security reports and coordinating
	the response*

Monthly meetings operators:

* West of GMT: Tianon Gravi <admwiggin@gmail.com>
* East of GMT: Sven Dowideit <svendowideit@home.org.au>

Infrastructure operators:

* Jessie Frazelle <jessie@docker.com>
* Michael Crosby <mike@docker.com>


### Subsystems

As the project grows, it gets separated into well-defined subsystems. Each subsystem
has a dedicated group of maintainers, which are responsible for its quality
This "cellular division" is the primary mechanism for scaling maintenance of the
project as it grows.

The maintainers of each subsytem are responsible for:

1. Exposing a clear road map for improving their subsystem.
2. Deliver prompt feedback and decisions on pull requests affecting their subsystem.
3. Be available to anyone with questions, bug reports, criticism etc.
  on their component. This includes IRC, GitHub requests and the mailing
  list.
4. Make sure their subsystem respects the philosophy, design and
  road map of the project.

The canonical list of subsystems, and the maintainers assigned to each, can be found in
SUBSYSTEMS.


#### How to review patches to your subsystem

Accepting pull requests:

  - If the pull request appears to be ready to merge, give it a `LGTM`, which
    stands for "Looks Good To Me".
  - If the pull request has some small problems that need to be changed, make
    a comment adressing the issues.
  - If the changes needed to a PR are small, you can add a "LGTM once the
    following comments are adressed..." this will reduce needless back and
    forth.
  - If the PR only needs a few changes before being merged, any MAINTAINER can
    make a replacement PR that incorporates the existing commits and fixes the
    problems before a fast track merge.

Closing pull requests:

  - If a PR appears to be abandoned, after having attempted to contact the
    original contributor, then a replacement PR may be made.  Once the
    replacement PR is made, any contributor may close the original one.
  - If you are not sure if the pull request implements a good feature or you
    do not understand the purpose of the PR, ask the contributor to provide
    more documentation.  If the contributor is not able to adequately explain
    the purpose of the PR, the PR may be closed by any MAINTAINER.
  - If a MAINTAINER feels that the pull request is sufficiently architecturally
    flawed, or if the pull request needs significantly more design discussion
    before being considered, the MAINTAINER should close the pull request with
    a short explanation of what discussion still needs to be had.  It is
    important not to leave such pull requests open, as this will waste both the
    MAINTAINER's time and the contributor's time.  It is not good to string a
    contributor on for weeks or months, having them make many changes to a PR
    that will eventually be rejected.


### I'm a maintainer, and I'm going on holiday

Please let your co-maintainers and other contributors know by raising a pull
request that comments out your `MAINTAINERS` file entry using a `#`.

### I'm a maintainer. Should I make pull requests too?

Yes. Nobody should ever push to master directly. All changes should be
made through a pull request.

### How is this process changed?

Just like everything else: by making a pull request :)
