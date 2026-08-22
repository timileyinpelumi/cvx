import { redirect } from "next/navigation";

/** Profile used to be its own destination, which put two near-identical
 *  entries in the nav and two near-identical icons next to each other. The
 *  details now live on the account page; this keeps old links working. */
export default function ProfileRedirect() {
  redirect("/account");
}
