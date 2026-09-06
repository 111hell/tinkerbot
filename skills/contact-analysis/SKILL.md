---
name: contact-analysis
description: Find a person in the current user's SIU contacts and answer questions using the returned contact profile.
---
# SIU Contact Analysis

Use this skill when the user asks about a person in their SIU contacts and the answer depends on that person's profile.

## Instructions

1. Extract the contact name, alias, remark, or username from the user's query. If there is no usable search term, ask the user to clarify instead of searching broadly.
2. Call `siu_search_contacts` with that search term. The tool automatically searches contacts owned by the real sender of the current private message; never ask for or invent an owner user ID.
3. If there are no matches, say that no matching contact was found. Do not invent a profile.
4. If multiple results could refer to different people, present only the minimum identifying information needed to disambiguate, such as display name and username, and ask the user which person they mean.
5. For a clear match, answer the original query using only fields present in the tool result. Distinguish facts returned by SIU from your analysis, and state when the available profile does not support a conclusion.
6. Do not expose internal user IDs, raw tool responses, or fields unrelated to the user's question unless the user explicitly needs them.
7. Do not send, edit, revoke, or otherwise operate SIU messages. Produce the answer normally; the fixed SIU event handler owns reply delivery.
