package hair.zhy.fastcopy;

import java.net.InetAddress;

public final class DnsResolver {
    public static void main(String[] args) {
        if (args.length != 1) {
            System.err.println("usage: DnsResolver <hostname>");
            System.exit(2);
        }
        try {
            InetAddress[] addresses = InetAddress.getAllByName(args[0]);
            for (InetAddress address : addresses) {
                System.out.println(address.getHostAddress());
            }
        } catch (Throwable error) {
            System.err.println("DNS lookup failed: " + error);
            System.exit(1);
        }
    }
}
