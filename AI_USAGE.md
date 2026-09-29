# AI usage

## Which AI tools I used

Cursor, with the Grok agent.

## What I used AI for

- Turning the requirements into a plan (PRD and TSD).
- Writing the code.
- Discussing edge cases and how to handle them.

## One place where AI helped me move faster

- It generated a lot of the code quickly.
- It helped me compare scenarios and pick the ones that were more reliable and cheaper to build.

## One place where I disagreed with, corrected, or rejected AI output

For the typical "last seat booking" problem, the AI strongly recommended the usual approach. It tended to go for the simple way. I did not accept that as the final answer, because the simple approach is weaker when two parents compete for the last seat.

## What I would change about my AI workflow if I did this again

I would challenge the AI earlier and push it to look for the best solution, not just the first common one.

## How I verified the final implementation

I tested it manually, even though unit tests were generated. That covered the last-seat race and the failed payment case.
