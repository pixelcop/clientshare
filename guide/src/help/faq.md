# Frequently asked questions

## Do recipients need an account?

Not for a valid secure link. The link grants access according to its permissions and expiry. An invitation creates a named account for ongoing access.

## What is the difference between a client and a user?

A client is a document workspace. A user is a person who signs in. One client can have multiple users, and a client user can be assigned to multiple clients.

## How are Internal, External and access roles different?

**Internal** identifies team members; **External** identifies clients and advisers.
Internal people have an organization role of **Member** or **Administrator**.
Members and External people need document assignments; an account alone grants
no document access. Editable access roles such as Viewer and Contributor control
what they can do in a workspace, folder or file. See [Roles and access](../getting-started/roles-and-access.md).

## Can I share only one folder with a secure link?

Yes. Select the folder's **Share** action and create a link there. It includes
that folder and everything inside it. To share the entire client, use **Share workspace**
at the client workspace root, or the client's **Share** action in **Clients**.

## Can upload-only recipients download or delete files?

No. They can upload files, but cannot see existing files or folders. Upload-only
links do not allow downloads or deletion.

## Can I extend a link or change its permissions?

Create a new link with the desired settings and send it to the recipient. Revoke the old active link if it should no longer be used. There is no edit action in the current link list.

## How do I stop sharing?

Remove the relevant account assignment and select **Save**, and revoke any secure
links the person may hold. Check inherited and additional access too: permissions
add across sources. Neither action removes copies already downloaded.

## Does uploading a file with the same name replace it?

Do not assume replacement. The upload flow can ask for distinct names when a conflict is detected. Use a clear version name and tell the recipient which document to use.

## Can I recover deleted files?

The app has no user-facing restore workflow. Ask the portal operator about their backup and recovery arrangements.

## Why are some Settings fields missing?

Settings requires an administrator account. Public base URL and runtime log level are single-tenant controls and are not shown for hosted workspaces. The current UI also does not expose a primary-color editor.

## Does a link's last-used time mean the document was approved?

No. It records link access where available, not document approval or proof that every file was read. Confirm approvals through your team's normal process.

## Where can I find self-hosting instructions?

See the project's [README](https://github.com/pixelcop/clientshare#quick-start) for installation, configuration, and build instructions. This guide covers using the portal after it is running.
